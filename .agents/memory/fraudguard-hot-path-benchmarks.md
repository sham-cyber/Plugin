---
name: FraudGuard hot-path benchmarks
description: Performance guidance for bounded detector histories and interpreting local benchmarks.
---

**Rule:** Keep bounded history eviction allocation-light after profiles reach their cap. Report in-process engine and guard benchmarks separately from end-to-end gateway latency; never infer an HTTP/provider SLO from them.

**Why:** A local benchmark exposed avoidable repeated allocation when rolling histories reached their configured size. The improved measurements covered only the detector and context guard, not Bifrost HTTP, Redis, provider/network time, or deployment scheduling.

**How to apply:** When changing rolling history bounds or hot-path scoring, rerun allocation-aware benchmarks and tests. State the measured scope and environment, and do not turn local microbenchmark results into a production latency guarantee.
