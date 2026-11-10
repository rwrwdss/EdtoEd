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

Organization scoping isolates data, while PostgreSQL connections and the ONNX worker are shared. Before setting a latency objective for a shared deployment, test longer runs on the target infrastructure and consider per-organization admission limits. These short loopback measurements exclude TLS, ingress and WAN latency and are not a capacity guarantee or a REST-versus-gRPC ranking.
