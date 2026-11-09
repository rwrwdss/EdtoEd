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

The current training pipeline needs a leakage audit: assessment submissions are not filtered by submission date, and model variants were compared on the test split. Published metrics are exploratory, not an independent production-quality estimate. The target is academic failure/withdrawal, not measured burnout. The API still uses a temporary cohort median of 1; production validity requires real cohort normalization.

See [backend documentation](edtoed/README.md) for endpoints and configuration. Its native predictor description is superseded by the runtime integration above.
