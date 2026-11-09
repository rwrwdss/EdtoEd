"""Train the first EdtoEd baseline on OULAD.

The target is an early-warning proxy: Fail or Withdrawn = 1, Pass or
Distinction = 0.  Features are restricted to the first 28 course days so the
evaluation does not use the final outcome period.  No third-party ML package
is required; the small logistic-regression solver is deliberately explicit so
the experiment is reproducible in the repository's lightweight environment.
"""

from __future__ import annotations

import argparse
import json
import hashlib
import onnxruntime as ort
import zipfile
from pathlib import Path

import numpy as np
import onnx
import pandas as pd
from onnx import TensorProto, helper, numpy_helper


FEATURES = ["relative_clicks", "test_score", "deadline_delay"]


def transform_features(frame: pd.DataFrame) -> np.ndarray:
    """Robustly compress heavy-tailed click and delay features."""
    raw = frame[FEATURES].to_numpy(float)
    return np.c_[np.tanh(raw[:, 0] / 2.0), raw[:, 1], np.tanh(raw[:, 2] / 5.0)]


def load_csv(root: Path, name: str) -> pd.DataFrame:
    return pd.read_csv(root / name)


def build_table(root: Path, chunk_size: int = 500_000) -> pd.DataFrame:
    students = load_csv(root, "studentInfo.csv")
    students = students[students["final_result"].isin(["Fail", "Withdrawn", "Pass", "Distinction"])].copy()
    students["target"] = students["final_result"].isin(["Fail", "Withdrawn"]).astype(np.int64)
    keys = ["id_student", "code_module", "code_presentation"]
    registrations = load_csv(root, "studentRegistration.csv")
    registrations["date_unregistration"] = pd.to_numeric(registrations["date_unregistration"], errors="coerce")
    registrations["date_registration"] = pd.to_numeric(registrations["date_registration"], errors="coerce")
    eligible = registrations[(registrations.date_unregistration.isna() | (registrations.date_unregistration > 28)) & (registrations.date_registration <= 28)]
    students = students.merge(eligible[keys], on=keys, validate="one_to_one")

    assessments = load_csv(root, "assessments.csv")
    assessments["date"] = pd.to_numeric(assessments["date"], errors="coerce")
    assessment_window = assessments[assessments["date"] <= 28][["id_assessment", "code_module", "code_presentation", "date"]]
    submissions = load_csv(root, "studentAssessment.csv")
    submissions["score"] = pd.to_numeric(submissions["score"], errors="coerce")
    submissions["date_submitted"] = pd.to_numeric(submissions["date_submitted"], errors="coerce")
    submissions = submissions[(submissions.date_submitted <= 28) & (submissions.is_banked == 0)]
    submissions = submissions.merge(assessment_window, on="id_assessment", how="inner")
    score = submissions.groupby(["id_student", "code_module", "code_presentation"], as_index=False).agg(
        test_score=("score", lambda s: float(np.clip(s.mean() / 100.0, 0.0, 1.0))),
        deadline_delay=("date_submitted", lambda s: 0.0),
    )
    delay_rows = submissions.copy()
    delay_rows["delay"] = delay_rows["date_submitted"] - delay_rows["date"]
    delays = delay_rows.groupby(["id_student", "code_module", "code_presentation"], as_index=False)["delay"].mean()
    score = score.drop(columns=["deadline_delay"]).merge(
        delays.rename(columns={"delay": "deadline_delay"}),
        on=["id_student", "code_module", "code_presentation"],
        how="left",
    )

    clicks = []
    vle_path = root / "studentVle.csv"
    for chunk in pd.read_csv(vle_path, chunksize=chunk_size):
        chunk = chunk[(chunk["date"] >= 0) & (chunk["date"] <= 28)]
        if not chunk.empty:
            clicks.append(chunk.groupby(["id_student", "code_module", "code_presentation"], as_index=False)["sum_click"].sum())
    click_table = pd.concat(clicks, ignore_index=True)
    click_table = click_table.groupby(["id_student", "code_module", "code_presentation"], as_index=False)["sum_click"].sum()
    click_table = click_table.rename(columns={"sum_click": "clicks"})
    table = students[["id_student", "code_module", "code_presentation", "target"]].merge(
        click_table, on=keys, how="left", validate="one_to_one"
    ).merge(score, on=keys, how="left", validate="one_to_one")
    table["clicks"] = table["clicks"].fillna(0)
    table = normalize_cohort(table)
    table["test_score"] = table["test_score"].fillna(0.0)
    table["deadline_delay"] = table["deadline_delay"].fillna(0.0)
    return table


def normalize_cohort(table: pd.DataFrame) -> pd.DataFrame:
    """Match API: include submitted zeros; reject count<2 or zero median."""
    table = table.copy()
    group = table.groupby(["code_module", "code_presentation"])["clicks"]
    table["median_clicks"] = group.transform("median")
    table["cohort_count"] = group.transform("size")
    table["scorable"] = (table.median_clicks > 0) & (table.cohort_count >= 2)
    table["relative_clicks"] = table.clicks / table.median_clicks.where(table.scorable)
    return table


def split_by_student(table: pd.DataFrame) -> tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    # Temporal evaluation, excluding students already observed in earlier splits.
    train = table[table.code_presentation.str.startswith("2013")]
    valid = table[(table.code_presentation == "2014B") & ~table.id_student.isin(train.id_student)]
    test = table[(table.code_presentation == "2014J") & ~table.id_student.isin(pd.concat([train, valid]).id_student)]
    assert all(len(frame) > 0 for frame in (train, valid, test))
    return train, valid, test


def sigmoid(z: np.ndarray) -> np.ndarray:
    return 1.0 / (1.0 + np.exp(-np.clip(z, -40, 40)))


def fit_logistic(x: np.ndarray, y: np.ndarray, epochs: int = 2500, lr: float = 0.08) -> tuple[np.ndarray, float, np.ndarray, np.ndarray]:
    mean, scale = x.mean(axis=0), x.std(axis=0)
    scale[scale < 1e-8] = 1.0
    z = (x - mean) / scale
    w, b = np.zeros(z.shape[1]), 0.0
    positive_weight = (len(y) - y.sum()) / max(y.sum(), 1)
    for _ in range(epochs):
        p = sigmoid(z @ w + b)
        error = (p - y) * np.where(y == 1, positive_weight, 1.0)
        w -= lr * (z.T @ error / len(y) + 0.002 * w)
        b -= lr * error.mean()
    return w, float(b), mean, scale


def calibrate(logits: np.ndarray, y: np.ndarray) -> tuple[float, float]:
    """Fit a monotone Platt map on calibration students only."""
    z = (logits - logits.mean()) / max(logits.std(), 1e-8)
    slope, intercept = 0.0, 0.0
    for _ in range(4000):
        error = sigmoid(slope * z + intercept) - y
        slope -= .05 * (np.mean(error * z) + .001 * slope)
        intercept -= .05 * np.mean(error)
    a = slope / max(logits.std(), 1e-8)
    c = intercept - a * logits.mean()
    if a <= 0:
        raise ValueError("Calibration reversed risk ordering")
    return float(a), float(c)


def choose_threshold(y: np.ndarray, p: np.ndarray) -> tuple[float, dict]:
    """Maximize F2 subject to precision >= 0.6 on threshold students."""
    candidates = np.unique(np.r_[.05, .95, np.quantile(p, np.linspace(0, 1, 501))])
    best = None
    for threshold in candidates:
        pred = p >= threshold
        tp = np.sum(pred & (y == 1))
        precision = tp / max(pred.sum(), 1)
        recall = tp / max(np.sum(y == 1), 1)
        if precision < .6:
            continue
        f2 = 5 * precision * recall / max(4 * precision + recall, 1e-12)
        candidate = (f2, recall, -threshold)
        if best is None or candidate > best[0]:
            best = candidate, float(threshold), {"precision": float(precision), "recall": float(recall), "f2": float(f2)}
    if best is None:
        raise ValueError("No threshold meets the precision floor")
    return best[1], best[2]


def metrics(y: np.ndarray, p: np.ndarray, threshold: float = 0.5) -> dict[str, float]:
    pred = (p >= threshold).astype(int)
    tp, tn = int(((pred == 1) & (y == 1)).sum()), int(((pred == 0) & (y == 0)).sum())
    fp, fn = int(((pred == 1) & (y == 0)).sum()), int(((pred == 0) & (y == 1)).sum())
    precision = tp / max(tp + fp, 1)
    recall = tp / max(tp + fn, 1)
    positives = max(int(y.sum()), 1)
    negatives = max(int(len(y) - y.sum()), 1)
    ranks = pd.Series(p).rank(method="average").to_numpy()
    auc = float((ranks[y == 1].sum() - positives * (positives + 1) / 2) / (positives * negatives))
    return {"n": int(len(y)), "positive_rate": float(y.mean()), "accuracy": float((pred == y).mean()),
            "precision": float(precision), "recall": float(recall), "f1": float(2 * precision * recall / max(precision + recall, 1e-12)), "roc_auc": auc,
            "brier": float(np.mean((p-y)**2)), "tp": tp, "tn": tn, "fp": fp, "fn": fn}


def export_onnx(path: Path, w: np.ndarray, b: float, mean: np.ndarray, scale: np.ndarray) -> None:
    # x' = [tanh(x0/2), x1, tanh(x2/5)]; z = (x' - mean) / scale.
    # p1 = sigmoid(z @ w + b); p0 = 1 - p1.
    weight = (w / scale).astype(np.float32).reshape(3, 1)
    bias = np.array([b - np.dot(mean / scale, w)], dtype=np.float32)
    indices0 = numpy_helper.from_array(np.array([0], dtype=np.int64), "feature_0")
    indices1 = numpy_helper.from_array(np.array([1], dtype=np.int64), "feature_1")
    indices2 = numpy_helper.from_array(np.array([2], dtype=np.int64), "feature_2")
    nodes = [
        helper.make_node("Gather", ["float_input", "feature_0"], ["clicks"], axis=1),
        helper.make_node("Gather", ["float_input", "feature_1"], ["score"], axis=1),
        helper.make_node("Gather", ["float_input", "feature_2"], ["delay"], axis=1),
        helper.make_node("Constant", [], ["click_scale"], value=numpy_helper.from_array(np.array([2.0], dtype=np.float32))),
        helper.make_node("Constant", [], ["delay_scale"], value=numpy_helper.from_array(np.array([5.0], dtype=np.float32))),
        helper.make_node("Div", ["clicks", "click_scale"], ["clicks_scaled"]),
        helper.make_node("Div", ["delay", "delay_scale"], ["delay_scaled"]),
        helper.make_node("Tanh", ["clicks_scaled"], ["clicks_transformed"]),
        helper.make_node("Tanh", ["delay_scaled"], ["delay_transformed"]),
        helper.make_node("Concat", ["clicks_transformed", "score", "delay_transformed"], ["transformed"], axis=1),
        helper.make_node("MatMul", ["transformed", "weight"], ["linear"], name="linear_features"),
        helper.make_node("Add", ["linear", "bias"], ["logit"], name="linear_bias"),
        helper.make_node("Sigmoid", ["logit"], ["positive_probability"], name="sigmoid"),
        helper.make_node("Constant", [], ["one"], value=numpy_helper.from_array(np.array([1.0], dtype=np.float32))),
        helper.make_node("Sub", ["one", "positive_probability"], ["negative_probability"], name="negative_probability"),
        helper.make_node("Concat", ["negative_probability", "positive_probability"], ["probability_tensor"], axis=1),
        helper.make_node("ArgMax", ["probability_tensor"], ["label_raw"], axis=1, keepdims=0),
        helper.make_node("Cast", ["label_raw"], ["label"], to=TensorProto.INT64),
        helper.make_node("ZipMap", ["probability_tensor"], ["probabilities"], domain="ai.onnx.ml", classlabels_int64s=[0, 1]),
    ]
    probabilities_type = onnx.TypeProto()
    probabilities_type.sequence_type.elem_type.map_type.key_type = TensorProto.INT64
    probabilities_type.sequence_type.elem_type.map_type.value_type.tensor_type.elem_type = TensorProto.FLOAT
    probabilities_output = helper.make_value_info("probabilities", probabilities_type)
    graph = helper.make_graph(nodes, "edtoed_oulad_logistic", [helper.make_tensor_value_info("float_input", TensorProto.FLOAT, [None, 3])],
                              [helper.make_tensor_value_info("label", TensorProto.INT64, [None]), probabilities_output],
                              initializer=[numpy_helper.from_array(weight, "weight"), numpy_helper.from_array(bias, "bias"), indices0, indices1, indices2])
    model = helper.make_model(graph, ir_version=7, opset_imports=[helper.make_operatorsetid("", 13), helper.make_operatorsetid("ai.onnx.ml", 3)], producer_name="edtoed-oulad-baseline")
    onnx.checker.check_model(model)
    onnx.save(model, path)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--data", type=Path, default=Path("data/raw/oulad"))
    parser.add_argument("--artifact-dir", type=Path, default=Path("artifacts"))
    parser.add_argument("--zip", type=Path, default=Path("data/raw/oulad.zip"))
    args = parser.parse_args()
    if not (args.data / "studentInfo.csv").exists():
        args.data.mkdir(parents=True, exist_ok=True)
        with zipfile.ZipFile(args.zip) as zf:
            zf.extractall(args.data)
    table = build_table(args.data)
    coverage = {"total": len(table), "scorable": int(table.scorable.sum()), "zero_click_students": int((table.clicks == 0).sum())}
    train, valid, test = split_by_student(table[table.scorable])
    w, b, mean, scale = fit_logistic(transform_features(train), train.target.to_numpy(float))
    # Disjoint student subsets within 2014B: calibration then policy selection.
    ids = np.random.default_rng(2026).permutation(valid.id_student.unique())
    calibration = valid[valid.id_student.isin(ids[:len(ids)//2])]
    selection = valid[~valid.id_student.isin(ids[:len(ids)//2])]
    def raw_logit(frame):
        return ((transform_features(frame) - mean) / scale) @ w + b
    a, c = calibrate(raw_logit(calibration), calibration.target.to_numpy())
    calibrated_w, calibrated_b = a*w, a*b+c
    def predict(frame):
        return sigmoid(a*raw_logit(frame)+c)
    threshold, selected = choose_threshold(selection.target.to_numpy(), predict(selection))
    args.artifact_dir.mkdir(parents=True, exist_ok=True)
    candidate_path = args.artifact_dir / "edtoed_oulad_v1.onnx"
    export_onnx(candidate_path, calibrated_w, calibrated_b, mean, scale)
    options = ort.SessionOptions()
    options.intra_op_num_threads = 1
    candidate = ort.InferenceSession(str(candidate_path), sess_options=options)
    incumbent_path = Path("../models/edtoed_v1.onnx")
    incumbent = ort.InferenceSession(str(incumbent_path), sess_options=options)
    def run(session, frame):
        x = frame[FEATURES].to_numpy(np.float32)
        return np.array([row[1] for row in session.run(["probabilities"], {"float_input": x})[0]])
    for frame in (calibration, selection, test):
        np.testing.assert_allclose(run(candidate, frame), predict(frame), atol=1e-6)
    report = {
        "dataset": "OULAD", "target": "Fail or Withdrawn", "window_days": 28,
        "protocol": "2013 train; disjoint 2014B calibration and threshold selection students; unseen 2014J test",
        "normalization": "all eligible roster including zero clicks; signed delays",
        "coverage": coverage, "features": FEATURES,
        "calibration": {"method": "Platt", "n": len(calibration), "a": a, "c": c,
                        "brier_before": metrics(calibration.target.to_numpy(), sigmoid(raw_logit(calibration)))["brier"],
                        "brier_after": metrics(calibration.target.to_numpy(), predict(calibration))["brier"]},
        "threshold_selection": {"n": len(selection), "objective": "max F2 with precision >= 0.6",
                                "threshold": threshold, **selected},
        "test": {
            "previous_at_0_7": metrics(test.target.to_numpy(), run(incumbent, test), .7),
            "candidate_at_0_7": metrics(test.target.to_numpy(), run(candidate, test), .7),
            "candidate_selected": metrics(test.target.to_numpy(), run(candidate, test), threshold),
        },
        "candidate_sha256": hashlib.sha256(candidate_path.read_bytes()).hexdigest(),
        "incumbent_sha256": hashlib.sha256(incumbent_path.read_bytes()).hexdigest(),
        "reference_predictions": [
            {"input": row, "risk": float(risk)}
            for row, risk in zip([[1,.8,0],[0,0,5],[0,.5,-2],[2,1,0]],
                                 predict(pd.DataFrame([[1,.8,0],[0,0,5],[0,.5,-2],[2,1,0]], columns=FEATURES)))
        ],
        "limitations": "Research dataset previously explored. No independent external validation. Threshold selected for recall with precision floor; interventions may increase."
    }
    (args.artifact_dir / "oulad_baseline_report.json").write_text(json.dumps(report, indent=2), encoding="utf-8")
    (args.artifact_dir / "oulad_baseline_report.md").write_text("# OULAD baseline\n\n```json\n" + json.dumps(report, indent=2) + "\n```\n", encoding="utf-8")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
