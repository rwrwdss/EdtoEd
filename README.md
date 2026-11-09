# EdtoEd

LMS analytics backend: REST, gRPC, PostgreSQL and real ONNX Runtime inference.

```sh
cd edtoed
docker compose up --build
```

Swagger: http://localhost:8080/docs

The backend loads `MODEL_PATH` once through a persistent Python ONNX Runtime worker (CPU, one thread). Startup fails if the model cannot execute. No hardcoded coefficient fallback is used by the server.

Local inference dependencies: `python3 -m pip install onnxruntime==1.22.1 numpy==2.2.6`. Set `PYTHON_BIN` to use a virtual environment interpreter.

Run integration tests: `cd edtoed && TEST_ONNX=1 go test -race ./...`.

Current contract: `float_input` float32 `[N,3]`, output `probabilities` with integer class key 1. Future models must preserve this contract or supply a new adapter. Requests are serialized per worker; worker failure requires a service restart.

## Research limitations

Training now filters submissions to day 28, excludes banked assessments and already withdrawn students, and evaluates 2013 → 2014B → 2014J with disjoint students. The new research candidate scores ROC-AUC 0.691, F1 0.573, accuracy 0.621 on 7,929 test enrollments. These figures supersede prior optimistic research claims, but refer to a different population and split. OULAD has already been explored, so external validation remains necessary.

The candidate remains in `edtoed/ml_research/artifacts/`; the serving model in `models/` has not been promoted or changed. Existing inference reference tests still apply to that serving model. Do not copy a research model into production without parity checks and acceptance criteria.

The target is academic failure/withdrawal, not measured burnout. The API still uses a temporary cohort median of 1; production validity requires real cohort normalization. Its event contract lacks cohort and observation-window identifiers, so pooling existing events would not reproduce training features.

See [backend documentation](edtoed/README.md) for endpoints and configuration. Its native predictor description is superseded by the runtime integration above.
