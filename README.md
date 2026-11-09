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

Training includes zero-click students in cohort medians, signed deadline delays, and no feature clipping. Unscorable cohorts (count<2 or median<=0) are excluded just as the API refuses scoring. All 27,515 eligible OULAD enrollments are scorable, including 1,203 zero-click students.

The promoted model in `edtoed/models/edtoed_v1.onnx` passed validation comparison and Python/ONNX/Go parity checks. Test ROC-AUC: 0.694, F1: 0.574, accuracy: 0.624 at threshold 0.5. Compared with the previous serving model on identical inputs, recall improved but precision, accuracy and test Brier worsened. At the actual service threshold 0.7, precision is 0.697 and recall 0.281 (F1 0.401). Threshold remains unchanged. OULAD has been explored previously; these are research results, not independent external validation.

The target is academic failure/withdrawal, not measured burnout. API now requires cohort and observation window; it computes the real median from per-student snapshots. See [cohort contract](edtoed/COHORT_API.md) for examples, retry semantics and model compatibility limits.

See [backend documentation](edtoed/README.md) for endpoints and configuration. Its native predictor description is superseded by the runtime integration above.
