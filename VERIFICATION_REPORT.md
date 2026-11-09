# FraudGuard verification report

**Report date:** 2026-11-04

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
- Independent, disabled-by-default payment velocity and new-coarse-country signals when a host supplies trusted payment events.
- Independent, disabled-by-default expired/revoked/exposed key-status signals when a trusted auth or credential system supplies those booleans.
- Optional cross-key signal using a resolved authenticated user identity, disabled by default.
- Thread-safe in-memory state, bounded profile and correlation storage, and process-local HMACs for internal identifiers.
- Observe-only hooks: findings do not short-circuit, reject, or change a request.
- Optional AI review, disabled by default, through a Bifrost-host-supplied adapter. It receives only a sanitized deterministic finding and returns a separate advisory; it cannot change the score or request flow.
- AI review interprets existing named signals only; it is not a separate model-based detector and cannot discover patterns absent from the rule-based signals.
- No prompt/payload retention, Redis, or provider-specific model client in the plugin. The host adapter owns any model call.

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

## 5. Privacy and scope checks

- Findings contain structured scores and fixed signal reasons, not prompt text, raw virtual-key IDs, or raw cohort IDs.
- The optional host-model adapter receives only score, threshold, alert/warmup flags, and fixed signals. It does not receive prompt/payload text, keys, payment IDs, provider/model labels, or credentials.
- AI advisories are separate from deterministic findings; model output cannot change scores or request handling.
- Profile and cohort map keys use process-local HMACs; the HMAC key is not persisted.
- Payment subject IDs and coarse country values are HMACed before retention. Findings do not include payment subjects, country values, payment amounts, or credential values.
- Cross-key correlation is opt-in and does not use IP or request headers.
- The demo is synthetic and makes no provider or network calls.
- The upstream checkout remained at the pinned commit with a clean worktree; no upstream source files were changed.

## 6. Not verified / not claimed

- FraudGuard is **not** installed, accepted, or merged into upstream Bifrost.
- The Bifrost HTTP server's built-in plugin registration/configuration path has not been wired or tested.
- No live Bifrost-host model adapter was connected or tested; the adapter contract was tested with an in-process test implementation.
- No live-provider, production, multi-replica, or real-customer traffic test was run.
- No payment provider or credential-status source is connected; optional payment and key-status signals only run when a trusted host explicitly supplies their inputs.
- No latency benchmark was run; no sub-30ms target is claimed.
- The Go plugin has no dashboard. A separate company-facing page is only a synthetic UI prototype, with no live plugin feed, tenant authentication, subscription/billing system, or email/SMS delivery. Redis/shared storage and automatic blocking are not implemented.
- Findings are heuristic indicators, not proof of fraud. False positives are possible and thresholds require review against representative, permissioned traffic.
- The project author supplied an image of Indian Patent Application No. 202521074434 (publication no. IN202521074434 A1). This report records the reference as author-provided only; it does not verify the application's legal status or whether its claims cover FraudGuard.
