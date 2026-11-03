# FraudGuard verification report

**Report date:** 2026-11-03

**Purpose:** Record what was inspected and tested for the FraudGuard hackathon prototype, and what has not been verified.

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

The prototype includes a compile-time interface assertion and hook tests. Its module uses Bifrost core API version `v1.14.1`, and validation also used a temporary Go workspace that pointed to the local upstream `core` source at the pinned commit.

Direct use through `core.Init` and `schemas.BifrostConfig.LLMPlugins` is documented in `plugins/fraudguard/README.md`. The upstream HTTP server has a separate built-in plugin loader; FraudGuard has not been added to that loader.

## 3. What the prototype implements

- Per-resolved-virtual-key rolling behavior profiles.
- Deterministic scores from 0 to 100 with named signals and reasons.
- Warmup, window, retention, profile-count, and alert-threshold configuration.
- Detection signals for model/provider novelty, rapid request sequences, input/output token spikes, and repeated failures.
- Optional cross-key signal using a resolved authenticated user identity, disabled by default.
- Thread-safe in-memory state, bounded profile and correlation storage, and process-local HMACs for internal identifiers.
- Observe-only hooks: findings do not short-circuit, reject, or change a request.
- No prompt/payload retention, Redis, external AI service, or network call in the detector.

## 4. Checks run

The checks below were run from `plugins/fraudguard` on 2026-11-03 with Go 1.27.3.

| Check | Result |
| --- | --- |
| `GOWORK=/tmp/fraudguard-validation.work GOSUMDB=sum.golang.org go test ./...` | Passed using the temporary workspace that links the plugin module to the local pinned Bifrost `core` source. |
| `GOSUMDB=sum.golang.org go test -race ./...` | Passed. |
| `GOSUMDB=sum.golang.org go vet ./...` | Passed; no diagnostics. |
| `GOSUMDB=sum.golang.org go run ./cmd/demo` | Passed; printed `score=30`, `alert=false`, `signals=[rapid_sequence]`; no network traffic. |

The test suite covers benign behavior, below-rate-limit anomalies, warmup, deterministic scoring, token spikes, repeated failures, concurrency, profile/cohort bounds and expiry, privacy, exemptions, alert-threshold behavior, and the observe-only plugin hooks. A test for alert-threshold clamping above 100 was added and run as part of the passing suite.

## 5. Privacy and scope checks

- Findings contain structured scores and fixed signal reasons, not prompt text, raw virtual-key IDs, or raw cohort IDs.
- Profile and cohort map keys use process-local HMACs; the HMAC key is not persisted.
- Cross-key correlation is opt-in and does not use IP or request headers.
- The demo is synthetic and makes no provider or network calls.
- The upstream checkout remained at the pinned commit with a clean worktree; no upstream source files were changed.

## 6. Not verified / not claimed

- FraudGuard is **not** installed, accepted, or merged into upstream Bifrost.
- The Bifrost HTTP server's built-in plugin registration/configuration path has not been wired or tested.
- No live-provider, production, multi-replica, or real-customer traffic test was run.
- No latency benchmark was run; no sub-30ms target is claimed.
- No dashboard, Redis/shared store, automatic blocking, SaaS billing, pricing, or revenue model is implemented.
- Findings are heuristic indicators, not proof of fraud. False positives are possible and thresholds require review against representative, permissioned traffic.
- The project author supplied an image of Indian Patent Application No. 202521074434 (publication no. IN202521074434 A1). This report records the reference as author-provided only; it does not verify the application's legal status or whether its claims cover FraudGuard.
