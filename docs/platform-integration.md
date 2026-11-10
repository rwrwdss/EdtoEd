# Platform integration

EdtoEd is a standalone scoring service for online schools, course platforms, and LMS products. Each platform keeps its enrollment records, activity events, and user experience. Its backend or data pipeline converts local data into the same EdtoEd snapshot contract and sends it over REST or gRPC. EdtoEd stores those snapshots and returns academic risk scores and advisory actions.

## Run the synthetic pilot

From the repository root, create a development key and start EdtoEd:

```sh
umask 077
export EDTOED_API_KEY="$(openssl rand -hex 32)"
digest="$(printf '%s' "$EDTOED_API_KEY" | openssl dgst -sha256 -r | cut -d ' ' -f 1)"
printf '[{"organization_id":"demo-org","sha256":"%s"}]\n' "$digest" > edtoed/keys.local.json
(cd edtoed && docker compose up --build -d)
```

When `/readyz` returns 200, run:

```sh
python3 examples/platform-pilot.py --url http://localhost:8080
```

The script needs only the Python standard library and a running EdtoEd instance. It writes three synthetic snapshots to a new demo cohort, including one learner with zero clicks. It then sends the same snapshots again to score against the complete cohort. Use a development or disposable database: the snapshots remain stored. No LMS credentials or student data are needed.

## Map platform data to the contract

| Platform data | EdtoEd field |
|---|---|
| Stable, pseudonymous learner identifier | `student_id` |
| Course or unit identifier at the scoring granularity | `topic_id` |
| Course run or group within the authenticated organization | `cohort_id` |
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

The operator issues a random 256-bit key to each platform backend. Keep the raw key in the platform's secret store. EdtoEd loads `EDTOED_KEYS_FILE` on startup: a JSON array of entries with a stable `organization_id` and `sha256` (the SHA-256 hex digest of the key string, with no newline). Different keys can map to the same organization; a digest cannot be assigned twice. To rotate keys, add the new digest under the same organization ID, restart, switch the client, then remove the old digest and restart again.

Send `Authorization: Bearer <key>` with REST requests, or lowercase `authorization` metadata with the same value in gRPC. Missing or invalid keys return HTTP 401 or gRPC `Unauthenticated`. The server takes the organization from the key; IDs in the request cannot select a different organization. Each organization has its own snapshots and cohort medians, even when IDs overlap. Health probes and API docs remain accessible without a key.

Use TLS at your HTTP/gRPC ingress and keep the backend ports private. Compose binds both published ports to localhost. This deployment shares the service and database; isolation is enforced on authenticated API calls, not between operators with direct database access. Previous snapshots without verified ownership remain in their original table and are excluded from scoring; re-submit them under the correct organization's key.

The demo verifies integration mechanics. To validate predictions before using them for real interventions, run a pilot on historical labeled outcomes from the platform and measure missed cases and false alerts at the serving threshold.
