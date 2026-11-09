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
import math
import os
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

    assessments = load_csv(root, "assessments.csv")
    assessments["date"] = pd.to_numeric(assessments["date"], errors="coerce")
    assessment_window = assessments[assessments["date"] <= 28][["id_assessment", "code_module", "code_presentation", "date"]]
    submissions = load_csv(root, "studentAssessment.csv")
    submissions["score"] = pd.to_numeric(submissions["score"], errors="coerce")
    submissions["date_submitted"] = pd.to_numeric(submissions["date_submitted"], errors="coerce")
    submissions = submissions.merge(assessment_window, on="id_assessment", how="inner")
    score = submissions.groupby(["id_student", "code_module", "code_presentation"], as_index=False).agg(
        test_score=("score", lambda s: float(np.clip(s.mean() / 100.0, 0.0, 1.0))),
        deadline_delay=("date_submitted", lambda s: 0.0),
    )
    delay_rows = submissions.copy()
    delay_rows["delay"] = np.maximum(delay_rows["date_submitted"] - delay_rows["date"], 0)
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
    cohort = click_table.groupby(["code_module", "code_presentation"])["clicks"].median().rename("median_clicks").reset_index()
    click_table = click_table.merge(cohort, on=["code_module", "code_presentation"], how="left")
    click_table["relative_clicks"] = click_table["clicks"] / click_table["median_clicks"].replace(0, 1)

    table = students[["id_student", "code_module", "code_presentation", "target"]].merge(
        click_table[["id_student", "code_module", "code_presentation", "relative_clicks"]],
        on=["id_student", "code_module", "code_presentation"], how="left",
    ).merge(score, on=["id_student", "code_module", "code_presentation"], how="left")
    table["relative_clicks"] = table["relative_clicks"].fillna(0.0).clip(0, 100)
    table["test_score"] = table["test_score"].fillna(0.0).clip(0, 1)
    table["deadline_delay"] = table["deadline_delay"].fillna(0.0).clip(0, 30)
    return table


def split_by_student(table: pd.DataFrame) -> tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    ids = table["id_student"].drop_duplicates().to_numpy()
    rng = np.random.default_rng(42)
    rng.shuffle(ids)
    n = len(ids)
    train_ids, valid_ids = set(ids[: int(n * 0.70)]), set(ids[int(n * 0.70): int(n * 0.85)])
    train = table[table.id_student.isin(train_ids)]
    valid = table[table.id_student.isin(valid_ids)]
    test = table[~table.id_student.isin(train_ids | valid_ids)]
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


def metrics(y: np.ndarray, p: np.ndarray) -> dict[str, float]:
    pred = (p >= 0.5).astype(int)
    tp, tn = int(((pred == 1) & (y == 1)).sum()), int(((pred == 0) & (y == 0)).sum())
    fp, fn = int(((pred == 1) & (y == 0)).sum()), int(((pred == 0) & (y == 1)).sum())
    precision = tp / max(tp + fp, 1)
    recall = tp / max(tp + fn, 1)
    order = np.argsort(-p)
    ys = y[order]
    positives = max(int(y.sum()), 1)
    negatives = max(int(len(y) - y.sum()), 1)
    tpr = np.cumsum(ys == 1) / positives
    fpr = np.cumsum(ys == 0) / negatives
    auc = float(np.trapezoid(np.r_[0, tpr], np.r_[0, fpr]))
    return {"n": int(len(y)), "positive_rate": float(y.mean()), "accuracy": float((pred == y).mean()),
            "precision": float(precision), "recall": float(recall), "f1": float(2 * precision * recall / max(precision + recall, 1e-12)), "roc_auc": auc}


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
    train, valid, test = split_by_student(table)
    w, b, mean, scale = fit_logistic(transform_features(train), train.target.to_numpy(float))
    def predict(frame: pd.DataFrame) -> np.ndarray:
        return sigmoid(((transform_features(frame) - mean) / scale) @ w + b)
    report = {"dataset": "OULAD", "target": "Fail or Withdrawn", "window_days": 28,
              "features": FEATURES, "rows": int(len(table)), "students": int(table.id_student.nunique()),
              "splits": {"train": metrics(train.target.to_numpy(), predict(train)), "validation": metrics(valid.target.to_numpy(), predict(valid)), "test": metrics(test.target.to_numpy(), predict(test))},
              "normalization_mean": mean.tolist(), "normalization_scale": scale.tolist(), "weights": w.tolist(), "bias": b}
    args.artifact_dir.mkdir(parents=True, exist_ok=True)
    (args.artifact_dir / "oulad_baseline_report.json").write_text(json.dumps(report, indent=2), encoding="utf-8")
    (args.artifact_dir / "oulad_baseline_report.md").write_text("# OULAD baseline\n\n```json\n" + json.dumps(report, indent=2) + "\n```\n", encoding="utf-8")
    export_onnx(args.artifact_dir / "edtoed_oulad_v1.onnx", w, b, mean, scale)
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
