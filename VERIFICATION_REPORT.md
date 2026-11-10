# FraudGuard verification report

**Report date:** 2026-11-10

**Purpose:** Record the pinned Bifrost inspection, module checks, Redis-backed product services, local server-loader overlay, and what has not been verified. This report does not claim a live production deployment.

## 1. Problem and upstream inspection

The target upstream source was inspected at commit [`54ad3abec4fbbdaedf59414415094760fb9aa7e9`](https://github.com/maximhq/bifrost/tree/54ad3abec4fbbdaedf59414415094760fb9aa7e9).

The inspected source includes:

- `core/schemas/plugin.go` for the LLM plugin interface.
- `plugins/governance/main.go` for request/token limits and governance hooks.
- `core/bifrost.go` for provider retry/key-rotation behavior.
- The upstream `plugins/` directory and its built-in plugin loader.

The inspected plugin list contains no `fraudguard` module. A source search found no dedicated behavior-based fraud/anomaly detector in the inspected `plugins/` or `core/` Go source. This is a bounded source inspection, not a claim that no custom plugin could ever be written.

The existing features are related but different: Governance handles configured request/token limits, and core key rotation is tied to provider failures/retries. The inspected source does not provide a dedicated per-virtual-key rolling behavior score for deviations such as new model/provider use, unusually rapid requests, or token-volume changes.

## 2. Plugin compatibility

At the inspected commit, `schemas.LLMPlugin` requires the base plugin methods and these hooks:

- `GetName()` and `Cleanup()`
- `PreRequestHook`
- `PreLLMHook`
- `PostLLMHook`

The module includes a compile-time interface assertion and hook tests. Its module uses Bifrost core API version `v1.14.1`; an earlier validation used a temporary Go workspace pointing to the local upstream `core` source at the pinned commit. The latest hardening checks below used the pinned core module version, not a live Bifrost server.

Direct use through `core.Init` and `schemas.BifrostConfig.LLMPlugins` is documented in `plugins/fraudguard/README.md`. A local overlay and guarded installer now add the plugin name and loader case to a separate Bifrost checkout at the pinned commit. This is a local integration patch only; it has not been pushed to or accepted by upstream.

## 3. What the module implements

- Per-resolved-virtual-key rolling behavior profiles.
- Deterministic scores from 0 to 100 with named signals and reasons.
- Warmup, window, retention, profile-count, and alert-threshold configuration.
- Detection signals for model/provider novelty, rapid request sequences, input/output token spikes, and repeated failures.
- Independent, disabled-by-default payment velocity and new-coarse-country signals when a host supplies trusted payment events.
- Independent, disabled-by-default expired/revoked/exposed key-status signals when a trusted auth or credential system supplies those booleans.
- Optional cross-key signal using a resolved authenticated user identity, disabled by default.
- Thread-safe in-memory state, bounded profile and correlation storage, and process-local HMACs for internal identifiers.
- Optional, default-off checks for common override/jailbreak phrases and a configured user-text byte threshold. Inspection is transient, capped, and limited to user text in supported text request types.
- A bounded asynchronous finding callback queue with queue-drop and callback-failure metrics; callback panics do not crash the process.
- Observe-only hooks: findings do not short-circuit, reject, or change a request.
- Optional AI review, disabled by default, through a Bifrost-host-supplied adapter. It receives only a sanitized deterministic finding and returns a separate advisory; it cannot change the score or request flow.
- AI review interprets existing named signals only; it is not a separate model-based detector and cannot discover patterns absent from the rule-based signals.
- Optional Redis tenant usage ledger with HMAC-pseudonymized tenant/event references, request/token/fraud-check latency aggregates, and deduplication by event ID. Gateway retry attempts use fresh hook IDs and are not deduplicated by this module.
- Operator-configured tier policy catalog for alert thresholds, payment velocity, feature entitlements, and usage-limit warning/overage signals. Policy outcomes are informational and never reject requests.
- Versioned usage export helper with no prices or blocked-request counts, plus HMAC-signed Redis Stream audit records and optional allowlisted HTTPS webhooks.
- No prompt/payload retention or provider-specific model client. Optional prompt checks inspect bounded user text transiently and retain only a boolean signal and byte count for the current scoring call. The host adapter owns any model call.

## 4. Checks run

The checks below were run from `plugins/fraudguard` on 2026-11-04 with Go 1.27.3. The exact-source run used a temporary Go workspace linking this module to `core` from the pinned upstream checkout.

| Check | Result |
| --- | --- |
| `BIFROST_ROOT=/path/to/pinned/bifrost bash scripts/verify-pinned-bifrost.sh` | Passed against commit `54ad3abec4fbbdaedf59414415094760fb9aa7e9`; the script ran tests, race detection, vet, and demo using local `core` source. |
| `GOSUMDB=sum.golang.org go test ./...` | Passed. |
| `GOSUMDB=sum.golang.org go test -race ./...` | Passed. |
| `GOSUMDB=sum.golang.org go vet ./...` | Passed; no diagnostics. |
| `GOSUMDB=sum.golang.org go run ./cmd/demo` | Passed; printed the synthetic request finding (`score=30`, `alert=false`, `signals=[rapid_sequence]`), payment finding (`score=95`, `alert=true`, `signals=[payment_new_location payment_velocity]`), and trusted key-status finding (`score=100`, `alert=true`, `signals=[key_expired key_exposed key_revoked]`). No network traffic. |

Additional tests for the optional AI-review adapter were run on 2026-11-09 with Go 1.27.3:

| Check | Result |
| --- | --- |
| `GOSUMDB=sum.golang.org go test ./...` | Passed. |
| `GOSUMDB=sum.golang.org go test -race ./...` | Passed. |
| `GOSUMDB=sum.golang.org go vet ./...` | Passed; no diagnostics. |
| `GOSUMDB=sum.golang.org go run ./cmd/demo` | Passed; standalone demo keeps AI review disabled and makes no network calls. |

The full suite now also tests privacy-safe AI inputs, score/alert independence, bounded asynchronous queue behavior, timeout/error handling, and concurrent reviews. These tests use a local test adapter; no live host model call was made. The suite also covers benign behavior, below-rate-limit anomalies, warmup, deterministic scoring, token spikes, repeated failures, request/payment concurrency, profile/cohort/payment-state bounds and expiry, privacy, exemptions, alert-threshold behavior, independent payment/key-status opt-ins, and observe-only plugin hooks.

### Additional prompt and callback hardening checks (2026-11-09)

| Check | Result |
| --- | --- |
| `GOSUMDB=sum.golang.org go test -count=1 ./...` | Passed. |
| `GOSUMDB=sum.golang.org go test -race -count=1 ./...` | Passed. |
| `GOSUMDB=sum.golang.org go vet ./...` | Passed; no diagnostics. |
| `GOSUMDB=sum.golang.org go run ./cmd/demo` | Passed; output remains synthetic and observe-only. |

These tests use synthetic text fixtures only. They check optional prompt detection, user-text-only scope, scan caps, absence of prompt text in findings/profile state, bounded callback delivery and queue-drop accounting, and panic containment for finding and AI callbacks. They do not validate prompt false-positive rates on representative traffic.

### Tenant services and local Bifrost loader overlay (2026-11-10)

| Check | Result |
| --- | --- |
| `GOSUMDB=sum.golang.org go test -count=1 ./...` | Passed after adding Redis metering, tier policy, usage export, audit/webhook services, and privacy/overflow tests. |
| `GOSUMDB=sum.golang.org go test -count=1 -race ./...` | Passed, including concurrency tests for the product queue and local webhook fixtures. |
| `GOSUMDB=sum.golang.org go vet ./...` | Passed; no diagnostics. |
| `GOSUMDB=sum.golang.org go run ./cmd/demo` | Passed; synthetic observe-only request/payment/key-status findings printed, with no network calls. |
| Integration overlay application | Passed on a temporary local clone at the pinned Bifrost commit; the script added the module, server loader registration, configuration entry, and pass-through test. It did not push or modify the public upstream repository. |
| `go test -p 1 ./bifrost-http/server` | Passed once on the pinned local overlay clone before the final internal module hardening. A refreshed-source rerun was stopped during linking after running without a result; no pass is claimed for that rerun. |

## 5. Privacy and scope checks

- Findings contain structured scores and fixed signal reasons, not prompt text, raw virtual-key IDs, or raw cohort IDs.
- Optional prompt checks inspect only user text, cap the amount scanned, and pass only a match boolean and byte count to scoring; the text is not retained or emitted.
- Finding callbacks are queued asynchronously with a fixed capacity; overflow and callback failures increment aggregate metrics. `Cleanup` drains queued events.
- The optional host-model adapter receives only score, threshold, alert/warmup flags, and fixed signals. It does not receive prompt/payload text, keys, payment IDs, provider/model labels, or credentials.
- AI advisories are separate from deterministic findings; model output cannot change scores or request handling.
- Profile and cohort map keys use process-local HMACs; the HMAC key is not persisted.
- Redis tenant and usage-event key references are HMAC-derived. The shared usage/audit data does not make detector profiles shared across replicas.
- Audit records are HMAC-signed and Redis-retained; this is tamper-evident, not immutable against Redis administrators. Webhook payloads contain only sanitized audit records.
- Payment subject IDs and coarse country values are HMACed before retention. Findings do not include payment subjects, country values, payment amounts, or credential values.
- Cross-key correlation is opt-in and does not use IP or request headers.
- The demo is synthetic and makes no provider or network calls.
- A temporary checkout was locally modified by the integration overlay for verification. No commit, pull request, or push to upstream was made.

## 6. Not verified / not claimed

- FraudGuard is **not** installed, accepted, or merged into upstream Bifrost.
- The built-in loader overlay is present and applied to a temporary local checkout. The server-package test passed before the final internal module hardening; a refreshed-source rerun did not produce a result and was stopped during linking. No final refreshed-source server build or upstream installation, acceptance, or merge is claimed.
- No live Bifrost-host model adapter was connected or tested; the adapter contract was tested with an in-process test implementation.
- No live Redis, webhook endpoint, Stripe/billing engine, production, multi-replica, or real-customer traffic test was run. Redis and webhooks were tested only with local fixtures.
- No payment provider or credential-status source is connected; optional payment and key-status signals only run when a trusted host explicitly supplies their inputs.
- No latency benchmark was run; no sub-30ms target is claimed.
- Behavioral profiles remain in-memory and process-local; only product usage and audit data can be shared through Redis. No live tenant-facing dashboard, tenant authentication, subscription service, or email/SMS delivery is connected.
- Usage metering is asynchronous and can drop events on bounded-queue overflow; it is not a financial source of truth. Usage export is a helper, not an invoice or Stripe integration.
- Payment velocity counts each host-supplied payment event. Payment retry deduplication must be done by the host and is not implemented here.
- Requests and payments are never blocked. There are no blocked-request counts in the export.
- Findings are heuristic indicators, not proof of fraud. False positives are possible and thresholds require review against representative, permissioned traffic.
- The project author supplied an image of Indian Patent Application No. 202521074434 (publication no. IN202521074434 A1). This report records the reference as author-provided only; it does not verify the application's legal status or whether its claims cover FraudGuard.

## 7. Runtime configuration and Bifrost core pipeline (2026-11-10)

- Added a versioned runtime config file loader and polling watcher. It applies bounded risk settings atomically, retains the last known good values after an invalid update, and reports reload status without returning config contents.
- `config.example.json` is covered by a validation test. Tests also cover unknown `blocklist` field rejection, exemptions, invalid/oversized files, watcher recovery, and concurrent reloads with requests.
- `main_test.go` initializes the real Bifrost core client with FraudGuard attached and sends a chat request through its plugin pipeline to a local OpenAI-compatible HTTP mock. The alerting prompt anomaly reaches the mock provider unchanged; the finding contains no prompt. This is not a test of Bifrost's inbound HTTP gateway.
- After these changes, `go test -count=1 ./...`, `go test -count=1 -race ./...`, `go vet ./...`, and `go run ./cmd/demo` passed in `plugins/fraudguard`.
- No request throttle, blocklist, or automatic blocking was added. Payment-velocity settings create alerts only. The pinned Bifrost server loader's refreshed-source rebuild remains unverified as described in section 6.

## 8. Observability, Redis fail-safe, diagnostics, and packaging (2026-11-10)

- Added aggregate-only Prometheus exposition and optional OpenTelemetry meter instruments for hook evaluations, completion/finding/alert counts, evaluation duration, observed risk-score distribution, and active process-local profiles/cohorts/payment subjects. No tenant, key, prompt, model, provider, request, or payment labels are emitted.
- Added an authenticated admin-handler constructor. It requires a host-supplied authorization callback and provides only aggregate metrics/status/health endpoints; no tenant inspection, penalty-box, block-log, or blocked-request endpoint exists.
- Added a bounded Redis circuit breaker around usage, plan, and audit operations. The environment constructor skips the Redis startup ping, plan warming is best-effort, and inference remains fail-open. Product events may be missing during Redis outages; fail-closed inference behavior is intentionally unsupported.
- Added Makefile targets for build, tests, race, vet, synthetic demo, Redis connectivity check, and pinned-source integration verification.
- `make verify` passed: build, tests, race detector, vet, and synthetic demo. The synthetic Redis connectivity-check failure path was tested against a closed localhost port; it returned a generic failure and did not print the Redis URL or credentials. `make help`, shell syntax checks, and `git diff --check` passed.
- OpenTelemetry was instrumented through the host meter API and tested with a no-op meter; no live OpenTelemetry SDK/exporter or collector was configured. Prometheus text generation and admin HTTP authorization were tested locally with unit tests.
- No live Redis, external metrics scrape, real admin server/authentication, or updated pinned Bifrost inbound HTTP server test was run. No upstream changes or push were made.

## 9. Trusted advisory inputs, webhook verification, and Compose harness (2026-11-10)

- Added an optional `ThreatIntelFeed` that accepts only trusted, already-resolved internal virtual-key IDs, immediately HMACs them, and keeps bounded expiring digests in process memory. A match adds a fixed advisory signal. It does not read IP addresses, user-agent strings, caller-controlled headers, or external feeds; it never blocks or drops a request.
- Added an optional `BudgetMonitor` for host-supplied token counts and trusted estimated cost in micro-USD. It returns warning/over-limit fields and aggregate metrics only. It does not estimate provider pricing, enforce caps, or interrupt requests; the bounded counters are process-local and reset on restart.
- Exposed HMAC-SHA256 webhook body signing and constant-time verification helpers. The signing key must be at least 32 bytes. Signature verification does not prevent replay; receivers must separately enforce event freshness and uniqueness.
- Added root `docker-compose.yml`, a local-only OpenAI-compatible mock provider, a pinned-source Bifrost build recipe, and a launcher that clones the exact inspected commit into an ignored temporary directory and applies the integration overlay only to that disposable clone. The Compose config includes synthetic virtual-key/provider configuration and local Redis.
- `make verify` passed after these changes: Go build, unit/integration-fixture tests, race detector, vet, and synthetic demo. The mock provider tests confirm fixed responses, request-size limits, and no prompt echo.
- `docker compose config`, launcher `bash -n`, and JSON parsing of both configs passed.
- The full container build and `docker compose up` were **not** run. The temporary Bifrost source clone, image pulls, full UI/Go build, and live containerized gateway behavior remain unverified. The synthetic Compose key and HMAC key are for local demonstration only.
- No upstream Bifrost commit, public repository, or pre-existing user checkout was changed by this work. No GitHub push was performed. The plugin remains observe-only: threat matches and budget overruns are advisory; no IP/user-agent fingerprinting, request blocks, payment blocks, or hard spend caps were added.

## 10. Context guard, read-only admin CLI, benchmarks, and judge walkthrough (2026-11-10)

- Added an opt-in context guard for bounded user-role text segments in the current request. It returns only fixed flags for staged override phrasing, disclosure-action/target pairs, and schema-marker/override combinations. It does not inspect assistant/system/developer text, retain prompt text, carry state across requests, enforce JSON schemas, or block.
- Added `cmd/fraud-admin` with authenticated aggregate-only `health`, `status`, and `metrics` GET commands. It rejects all other commands; there is no tenant unblocking, raw Redis-key inspection, tier-policy mutation, or threat-feed clearing. The host must mount `NewAdminHandler` behind its own auth; the pinned Bifrost overlay does not mount this handler.
- Added Go benchmarks for the engine start/complete path, parallel engine calls, and bounded context inspection. On this Replit Linux x86-64 environment (Xeon Platinum 8581C; `-benchtime=1s -cpu 1,4`), observations were:
  - Engine sequential: 16.8 µs/op at `-cpu 1`; 26.4 µs/op at `-cpu 4`.
  - Engine parallel: 5.8 µs/op at `-cpu 1`; 10.3 µs/op at `-cpu 4`.
  - Context inspection: 1.40 µs/op at `-cpu 1`; 1.61 µs/op at `-cpu 4`.
  These are in-process detector/guard measurements only. They exclude Bifrost HTTP handling, Redis, provider/network time, and deployment scheduling and do not establish a 5-ms end-to-end SLO.
- The benchmark exposed repeated allocation when bounded histories reached capacity. Eviction was changed to trim in place; sequential allocation fell from about 38 KB/op to 13.8 KB/op and parallel allocation from about 10.6 KB/op to 4.5 KB/op. The configured history caps and scoring behavior are unchanged.
- `make verify` passed after the additions: build, package tests, race detector, vet, and synthetic demo. Tests cover the Bifrost hook receiving context signals while returning the same request, prompt/key privacy, config reload, CLI authorization behavior, and default-off context inspection.
- Root Compose configuration, both launcher shell syntaxes, three JSON configs, and `git diff --check` passed. The full Bifrost image build, `docker compose up`, and `demo_walkthrough.sh --compose` were not run. No live metrics endpoint is queried by default; the walkthrough only calls the admin CLI if an authenticated host URL is supplied.
- No upstream commit or user checkout was changed and no Git push was made. The demo asserts pass-through rather than a block; all signals remain advisory.

## 11. CI, health probes, and submission package (2026-11-10)

- Added `.github/workflows/fraud-plugin-ci.yml` for pull requests and manual dispatch. It runs Go build/tests, race detector, `go vet`, benchmarks, checks the pinned Bifrost API in a disposable checkout, applies the integration overlay only there, and builds the Compose gateway and mock-provider images. The workflow has not yet run on GitHub.
- Added `RedisProductStore.HealthCheck` and `CheckHealth`. Health probes use a bounded Redis ping and report stable status codes only. By default, an optional Redis outage marks product health degraded but leaves inference readiness true. Hosts may opt to make Redis or a process-wide heap threshold affect readiness.
- Added authenticated `NewProbeHandler` routes for `/livez`, `/healthz`, and `/readyz`. These routes are not automatically mounted in Bifrost; the host must attach them to a private listener and provide authorization. The memory measure is process-wide, not FraudGuard-only.
- Added unit tests for healthy Redis ping, optional versus required Redis readiness, heap threshold behavior, authorization, and prompt-safe status output. `make verify` passed after these changes: build, tests, race detector, vet, and synthetic demo.
- Added `SUBMISSION.md` with architecture, setup/demo instructions, boundaries, monetization status, and recorded local benchmarks. It states that monetization helpers do not charge customers or guarantee revenue, and that local microbenchmarks do not establish an end-to-end SLO.
- The proposed IP/tenant blocklist API was not implemented. The plugin does not treat IP/user-agent headers as identity and does not block. Trusted resolved internal-key exemptions remain config-based; no Redis policy mutation endpoint was added.
- Compose configuration, launcher shell syntax, JSON configs, and `git diff --check` passed. A dedicated GitHub Actions YAML linter was unavailable in this environment. Full Docker image builds, Compose smoke mode, Kubernetes probes, and GitHub Actions execution remain unverified.
- No upstream changes were committed or pushed.

## 12. Tracing, fault injection, and root automation (2026-11-10)

- Added OpenTelemetry spans for Bifrost pre/post hooks, opt-in prompt/context inspection, deterministic risk scoring, and optional Redis product-service operations. Trace attributes contain only booleans, a user-text byte count, score/signal counts, and stable failure codes. Raw prompts, key values, tenant IDs, model/provider labels, payloads, and error text are excluded.
- The host must configure its global OpenTelemetry tracer provider to export spans. The module's default no-op provider creates no exported telemetry; this turn did not test a configured SDK exporter or collector.
- Added chaos tests showing that a Redis disconnect does not block the gateway hook or rewrite a simulated upstream timeout, and that a delayed optional usage ledger does not hold the post-hook. Tests use miniredis and an in-process fake; they do not simulate a Redis Cluster or real network packet loss.
- Added root `Makefile` shortcuts for build, tests, race, vet, chaos tests, local demo, Compose walkthrough, pinned-source integration, and verification. The root help, build, chaos, and local-demo targets passed.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, and `git diff --check` passed in `plugins/fraudguard`. The synthetic demo printed observe-only findings as expected.
- No temporary penalty box or cooldown was added because it would ban or isolate tenants and conflict with observe-only behavior. No Redis-backed behavioral velocity check exists; Redis remains limited to optional product services.
- The pinned-source container walkthrough, live Redis/cluster testing, external trace export, production traffic, and upstream integration were not run for this change. No upstream files were changed and no Git push was made.

## 13. Custom advisory rules, dashboard, load script, and contributor guide (2026-11-10)

- Added a bounded in-memory custom prompt-rule manager using Go's RE2 regular expressions. Rule sets are replaced atomically through trusted host code; a match emits one fixed advisory signal. The module does not load rules from Redis, expose a rule-update endpoint, or block requests.
- Added authenticated `GET /admin/fraudguard/dashboard`, a process-wide JSON snapshot built from existing aggregate metrics and health status. It excludes active-tenant identities/counts and blocked-injection totals. Hosts still must mount it behind authorization on a private listener.
- Added `plugins/fraudguard/scripts/load-test.js` and root `make fraudguard-load-test`. The script defaults to loopback, limits duration and VUs, and uses synthetic requests. `node --check` passed; k6 is not installed here, so no load test was run. It measures the whole gateway, not isolated plugin overhead.
- Added `CONTRIBUTING.md` describing privacy, observe-only boundaries, heuristic contributions, and validation commands.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, root build/demo/chaos/help targets, and `git diff --check` passed after these changes.
- The tests do not establish production performance, do not exercise a real Bifrost inbound gateway or Redis Cluster, and do not validate a live OpenTelemetry exporter. The Compose walkthrough and k6 traffic run were not performed.
- No upstream files were changed and no Git push was made. Rule matches remain advisory; there are no blocked-request or penalty-box actions.
