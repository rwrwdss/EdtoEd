# Service metrics

Scrape `GET /metrics` on the HTTP listener in Prometheus text format. Keep this route on the monitoring network or restrict it at the ingress. It reports aggregate service traffic and does not require a platform key.

| Metric | Labels | Meaning |
|---|---|---|
| `edtoed_analysis_requests_total` | `transport`, `code` | Completed analysis requests, including authentication failures. REST uses HTTP status codes; gRPC uses canonical status names. |
| `edtoed_analysis_in_flight` | `transport` | Analysis requests currently being handled. |
| `edtoed_analysis_duration_seconds` | `transport` | Server-side request duration, including authentication, storage and inference when reached. |
| `edtoed_analysis_stage_duration_seconds` | `stage` | Time spent in storage or inference. Storage includes PostgreSQL waits; inference includes time waiting for the ONNX worker. |

Both duration metrics are histograms. A request rejected before storage does not contribute to either stage; a snapshot saved without a score contributes only to storage. Health probes, docs and scrapes are excluded. All series are local to the process and reset on restart. Keys, payloads, learner IDs and organization IDs are never used as labels.

Organization concurrency rejections appear as `code="429"` on REST and `code="ResourceExhausted"` on gRPC. Track these outcomes separately from failures in admitted analyses. Latency histograms include rejected requests, so fast rejections can lower reported percentiles even when admitted requests are slower.

Request rate by transport:

```promql
sum by (transport) (rate(edtoed_analysis_requests_total[5m]))
```

Approximate p95 latency by transport, in seconds:

```promql
histogram_quantile(
  0.95,
  sum by (le, transport) (rate(edtoed_analysis_duration_seconds_bucket[5m]))
)
```

Use the stage histograms alongside in-flight requests to distinguish increased storage waits from increased inference waits. These aggregate metrics do not measure per-platform fairness. See the [local two-platform load report](load-report.md) for a controlled comparison.
