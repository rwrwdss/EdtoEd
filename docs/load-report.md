# Two-platform load test

One EdtoEd instance served two authenticated synthetic platforms. Platform A used REST and platform B used gRPC. Each platform had its own key and used the same cohort, topic and learner identifiers as the other. All requests exercised the real PostgreSQL write and median calculation and the active ONNX model.

## Method

- Apple M3 (8 cores, 16 GiB), Go 1.25.7, PostgreSQL 14.18 and ONNX Runtime 1.22.1. Server, database and load client ran on the same host over loopback or a local database socket.
- Each cohort contained 64 learners, including a zero-click learner. Both platforms were seeded before the measurement. Requests replaced existing snapshots, as in repeat scoring.
- The platform under observation sent 40 requests/s for eight seconds. The other platform, when active, ran 72 clients with one outstanding request each. Its traffic started one second before observation. Percentiles are client-side for successful requests; throughput counts successful requests started in the measurement window, including those drained immediately after it.
- The first run used one cohort per platform and measured each baseline before concurrent traffic. The second run seeded eight cohorts per platform and spread background traffic across all eight; baseline measurements followed concurrent traffic. The observation stream reused the same sequence in each pair.

## Results

| Profile | Observed platform | Background requests/s | p50, ms | p95, ms | p99, ms |
|---|---|---:|---:|---:|---:|
| One cohort, baseline | REST A | — | 3.68 | 5.71 | 7.05 |
| One cohort, gRPC B active | REST A | 5,130 | 0.45 | 2.40 | 2.66 |
| One cohort, baseline | gRPC B | — | 3.12 | 6.01 | 7.16 |
| One cohort, REST A active | gRPC B | 5,216 | 0.43 | 2.33 | 2.73 |
| Eight cohorts, baseline | REST A | — | 3.39 | 4.50 | 5.06 |
| Eight cohorts, gRPC B active | REST A | 4,811 | 14.72 | 21.83 | 30.29 |
| Eight cohorts, baseline | gRPC B | — | 3.55 | 4.62 | 5.66 |
| Eight cohorts, REST A active | gRPC B | 5,527 | 13.08 | 17.08 | 19.61 |

The observed platform completed all 320 requests at 40 requests/s in every window. There were no errors or timeouts during measurement. The expected incomplete-cohort responses during seeding were counted separately. The scrape confirmed both transports' counters, drained in-flight gauges and both stage histograms; it contained no learner or organization IDs.

The eight-cohort profile showed interference in both directions: REST p95 rose **4.85×**, and gRPC p95 rose **3.70×**. The single-cohort profile did not show a regression in this run. Its lower concurrent percentiles should not be treated as a speedup: baseline order, machine state and database serialization differ.

Across each full run, including preparation and drain, mean measured storage/inference times were **13.88/0.03 ms** with one cohort and **1.80/11.86 ms** with eight. That shift is consistent with writes waiting on the same cohort in the first run and reaching the shared inference queue in parallel in the second. The inference metric includes waiting and execution; it cannot attribute the time to model compute alone.

Organization scoping isolates data, while PostgreSQL connections and the ONNX worker are shared. These short loopback measurements exclude TLS, ingress and WAN latency.

## Follow-up with an organization concurrency limit

The follow-up compares `36026ba` with the new default of eight concurrent analyses per organization. The two binaries ran on the same host with the same database, model, credentials and synthetic workload. Each platform had eight cohorts of 64 learners. The test alternated binary order (before, after, after, before) and reset and seeded the temporary database between runs.

Each window measured six seconds after a one-second background lead-in. The probe scheduled 40 requests/s and the background generator scheduled 4,800 requests/s in batches of 24 every 5 ms, with at most 72 outstanding requests. When all 72 client slots were occupied, the generator skipped that scheduled request and counted it separately. Reported p95 ranges span two runs per version and direction, using only successful requests. Outcome counts sum those two runs.

| Probe / background | Probe p95 before, ms | Probe p95 after, ms | Probe errors or rejections |
|---|---:|---:|---:|
| REST A / gRPC B | 36.55–38.82 | 4.10–5.36 | 0/480 before, 0/480 after |
| gRPC B / REST A | 44.39–58.59 | 5.17–7.80 | 0/480 before, 0/480 after |

| Background | Version | Successful p95, ms | Successful | Limit rejections | Other errors | Skipped by generator |
|---|---|---:|---:|---:|---:|---:|
| gRPC B | Before | 44.92–50.96 | 26,411 | 0 | 0 | 31,189 |
| gRPC B | After | 4.21–5.06 | 18,513 | 39,025 | 0 | 62 |
| REST A | Before | 46.01–53.28 | 25,996 | 0 | 0 | 31,604 |
| REST A | After | 4.15–5.26 | 18,570 | 38,786 | 0 | 244 |

The generator scheduled 57,600 background requests per direction and version. Before the change, occupied client slots suppressed much of the scheduled traffic. After the change, EdtoEd quickly returned HTTP 429 or gRPC `ResourceExhausted` on about 68% of the scheduled background traffic. The control streams completed every request, and background throughput fell in exchange for lower latency. The counts distinguish offered demand, actual calls and completed analyses.

A separate check held all eight slots of one organization with REST calls. Its gRPC call was rejected, the other organization completed a gRPC analysis, and the first organization could call again after the held requests finished. The limit applies per EdtoEd instance. These results describe this burst pattern and local setup. Production retry pacing and network costs will change the observed rates.
