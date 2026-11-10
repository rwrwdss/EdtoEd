# Platform integration

EdtoEd is a standalone scoring service for online schools, course platforms, and LMS products. Each platform keeps its enrollment records, activity events, and user experience. Its backend or data pipeline converts local data into the same EdtoEd snapshot contract and sends it over REST or gRPC. EdtoEd stores those snapshots and returns academic risk scores and advisory actions.

## Run the synthetic pilot

Start EdtoEd with Docker Compose from `edtoed/`. Then, from the repository root, run:

```sh
python3 examples/platform-pilot.py --url http://localhost:8080
```

The script needs only the Python standard library and a running EdtoEd instance. It writes three synthetic snapshots to a new demo cohort, including one learner with zero clicks. It then sends the same snapshots again to score against the complete cohort. Use a development or disposable database: the snapshots remain stored. No LMS credentials or student data are needed.

## Map platform data to the contract

| Platform data | EdtoEd field |
|---|---|
| Stable, pseudonymous learner identifier | `student_id` |
| Course or unit identifier at the scoring granularity | `topic_id` |
| Course run or group, scoped by platform or organization | `cohort_id` |
| Shared, completed observation period | `window_start`, `window_end` in Unix seconds |
| Activity count over that period | `clicks_count`, including zero for inactive learners |
| Assessment result and submission timing | `test_score` in [0, 1], `deadline_delay` in days; negative means early |

Use the same observation period and scoring granularity for everyone in a cohort. For the closest match to the training setup, start with course-level activity from day 0 through day 28, using an exclusive end at the start of day 29. A platform with a different definition of a click, a different assessment schedule, or a shorter window should validate that mapping before relying on the scores.

## Score and consume the results

1. Submit the full roster, including zero-activity learners. Early responses can be provisional. HTTP `409` / gRPC `FailedPrecondition` can mean the snapshot was stored but the cohort does not yet support scoring.
2. After the roster is in place, repeat the same snapshots for learners you want to score. The same key replaces the stored aggregate. Keep writes to that cohort and window coordinated during scoring, since edits can change the median.
3. Map `burnout_risk` to **academic risk** in the platform. The field name is historical. Treat `OFFER_HINT` as a suggestion for the platform or educator to review; EdtoEd does not send hints or take actions on the learner's account.

The current endpoint writes and scores one snapshot per call. It has no separate batch submission or cohort finalization step. See the [API contract](../edtoed/COHORT_API.md) for persistence and timeout behavior and the [protobuf schema](../edtoed/api/proto/edtoed.proto) for gRPC.

## Deployment boundary

For a pilot, run a separate EdtoEd instance and database per platform or organization, with access restricted to its backend on a private network. The current API does not enforce authentication or tenant isolation. A namespaced cohort identifier avoids accidental collisions but does not authorize callers. A shared deployment needs authenticated organization identity and storage isolation.

The demo verifies integration mechanics. To validate predictions before using them for real interventions, run a pilot on historical labeled outcomes from the platform and measure missed cases and false alerts at the serving threshold.
