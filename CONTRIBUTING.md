# Contributing

Start with the [project overview](README.md) and the [cohort API contract](edtoed/COHORT_API.md). The API, storage, and model can all be exercised locally without an LMS.

## Changes and commits

- Keep behavior changes, model updates, and unrelated documentation edits in separate commits. Include the regression test with the fix it covers.
- Write a short subject in the imperative mood, such as `fix: honor request deadlines during inference`. If a change affects clients or operators, explain the previous behavior, the reason for the change, and how you verified it in the commit body or pull request.
- Use comments to record constraints that matter during maintenance: for example, why interrupted worker exchanges require a new process, or why a snapshot can survive an inference failure.
- Keep the English and Russian READMEs aligned. When behavior changes, update the API contract and OpenAPI document alongside the implementation.
- Edit `edtoed/api/proto/edtoed.proto` for gRPC contract changes and regenerate the Go bindings. Avoid editing generated files by hand.

## Before pushing

Run these checks from `edtoed/`:

```sh
TEST_ONNX=1 go test -race ./...
go vet ./...
(cd ml_research && python3 -m unittest test_training -v)
```

Use the Python dependencies listed in the [README](README.md#development). Set `TEST_DATABASE_URL` to a disposable PostgreSQL database to run the storage integration tests. The CI workflow also runs those tests and builds the Docker image.

For a model update, include the evaluation report, the selected threshold, and updated inference reference cases. Keep raw datasets and database credentials out of Git. Document the evaluation split and which metrics improved or regressed.
