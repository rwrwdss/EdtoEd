# Cohort aggregate contract

Both REST and gRPC require `cohort_id`, `window_start` and `window_end`.
Window timestamps are Unix seconds: [start,end), end must be in the past.
Identifiers are integration-defined, independent of LMS vendor. Namespace cohort IDs by organization/source. This naming convention does not replace authentication or tenant isolation.

Send one cumulative snapshot per student/topic/cohort/window. `clicks_count` is the total in that window, not an event delta. Retries replace the same row; the last committed snapshot wins. Submit zero-click students explicitly. Send scores in [0,1] and deadline delay in days from that same observation window.

Median includes all submitted students, including zeros, and the current snapshot. It is scoped to the exact cohort/topic/window. Even-sized cohorts use the mean of the central pair. A transactional advisory lock serializes updates within that scope. Existing legacy logs are retained but excluded because they lack cohort/window attribution.

The first snapshot and a zero-median cohort produce HTTP 409 / gRPC FailedPrecondition after persistence. Submit the rest of the cohort and retry scoring. Predictions may change while the cohort is being populated; full-roster completeness is the integrator's responsibility. There is no hidden denominator fallback.

Analysis has a five-second server deadline, including the database write and inference. If HTTP returns 504 or gRPC returns DeadlineExceeded, the snapshot may already be committed. Retry the same snapshot to request a new score without adding another row.

```json
{"student_id":"student-1","topic_id":"module-1","cohort_id":"orgA-course2026-group1","window_start":1704067200,"window_end":1706486400,"clicks_count":24,"test_score":0.72,"deadline_delay":1.5}
```

Swagger is served at `/docs` and the schema at `/openapi.yaml`. The model was trained on early course aggregates and calibrated on OULAD. Its accuracy and calibration on other window lengths or institutions still need external validation. The model uses inclusive cohort medians and signed delays. Missing assessments map to score=0 and delay=0 in training; integrators should reproduce this convention.
