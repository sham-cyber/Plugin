# FraudGuard: Bifrost behavior-detection plugin

This project contains a production-oriented, observe-only Go plugin module for the Bifrost AI gateway, plus a synthetic hackathon demo. It looks for request-behavior changes that fixed request-count limits can miss, with separate opt-in signals for trusted payment events, credential status, bounded prompt checks, and transient multi-turn context heuristics. It reports findings; it never blocks requests or payments. A local integration overlay and Compose smoke-demo path are provided, but this is not an upstream feature or a production deployment; shared multi-replica behavior, trusted external sources, and production workload verification remain outstanding.

## Project documents

The technical README includes setup, demo, tests, Bifrost integration, privacy limits, and limitations. The judge-facing scope and runbook are in [SUBMISSION.md](SUBMISSION.md):

[Open the full FraudGuard README](plugins/fraudguard/README.md)

[Open the verification report](VERIFICATION_REPORT.md)

[Read the contribution guide](CONTRIBUTING.md)

## Verification Report — What Was Checked and What It Proves

This section is the project's evidence summary. It distinguishes what was checked from what remains unverified.

### Upstream inspection

The Bifrost source was inspected at commit [`54ad3abec4fbbdaedf59414415094760fb9aa7e9`](https://github.com/maximhq/bifrost/tree/54ad3abec4fbbdaedf59414415094760fb9aa7e9). The inspected plugin list has no `fraudguard` module, and the inspected `plugins/` and `core/` Go source contain no dedicated per-virtual-key behavior-based fraud detector. Bifrost Governance provides configured request/token limits; core key rotation is tied to provider failures and retries. This finding is limited to the pinned source inspected.

### Plugin compatibility and scope

The module implements Bifrost's `schemas.LLMPlugin` hooks and includes a compile-time interface assertion and hook tests. It can be supplied through `core.Init` using `schemas.BifrostConfig.LLMPlugins`. A local overlay can register it in a separate checkout at the pinned commit for testing; this project does not modify the public Bifrost repository, and the plugin has not been installed, accepted, or merged upstream.

The module maintains bounded, in-memory, per-virtual-key behavior profiles and produces deterministic 0–100 scores with named signals. It covers model/provider novelty, rapid request sequences, token-volume spikes, repeated failures, and optional bounded phrase/size checks over user text. Separate opt-in signals are available for trusted payment-event velocity/location and trusted expired/revoked/exposed key-status inputs. Those inputs are not supplied automatically by Bifrost and all findings remain observe-only. Prompt text is scanned only when explicitly enabled, is never retained or emitted, and is excluded from cross-key correlation. Cross-key correlation is opt-in and does not use IP addresses or request headers. Optional AI review receives only a sanitized finding summary and cannot change the deterministic score or request handling.

### Checks run

These checks were run on 2026-11-04 with Go 1.27.3:

- `BIFROST_ROOT=/path/to/pinned/bifrost bash scripts/verify-pinned-bifrost.sh` — passed against commit `54ad3abec4fbbdaedf59414415094760fb9aa7e9`; the script runs tests, race detection, vet, and demo against local `core` source.
- `GOSUMDB=sum.golang.org go test ./...` — passed.
- `GOSUMDB=sum.golang.org go test -race ./...` — passed.
- `GOSUMDB=sum.golang.org go vet ./...` — passed with no diagnostics.
- `GOSUMDB=sum.golang.org go run ./cmd/demo` — passed; it printed synthetic request, payment, and key-status findings. The demo makes no network calls.

Additional checks for the optional AI-review adapter were run on 2026-11-09 with Go 1.27.3:

- `GOSUMDB=sum.golang.org go test ./...` — passed.
- `GOSUMDB=sum.golang.org go test -race ./...` — passed.
- `GOSUMDB=sum.golang.org go vet ./...` — passed with no diagnostics.
- `GOSUMDB=sum.golang.org go run ./cmd/demo` — passed; AI review is disabled in the standalone demo.

The AI adapter tests use an in-process test implementation. No live Bifrost model call was made. Tests cover benign baselines, anomalies below fixed rate limits, warmup, deterministic scoring, token spikes, repeated failures, request/payment/AI-review concurrency, state bounds and expiration, privacy, exemptions, threshold behavior, independent payment/key-status opt-ins, bounded AI queue behavior, and observe-only hooks.

### Prompt and callback hardening checks

These additional checks were run on 2026-11-09 with Go 1.27.3:

- `GOSUMDB=sum.golang.org go test -count=1 ./...` — passed.
- `GOSUMDB=sum.golang.org go test -race -count=1 ./...` — passed.
- `GOSUMDB=sum.golang.org go vet ./...` — passed with no diagnostics.
- `GOSUMDB=sum.golang.org go run ./cmd/demo` — passed; output remains observe-only and synthetic.

The tests cover optional transient prompt checks, user-text-only scope, scan caps, prompt privacy, bounded non-blocking finding callbacks, queue-drop metrics, callback panic isolation, and AI adapter/callback panic isolation. No real user prompt, live Bifrost gateway, or production traffic was used.

### Advisory extensions and local container harness (2026-11-10)

The current `make verify` run passed after adding bounded HMAC-only trusted internal-key indicators, an observe-only token/cost budget monitor, HMAC webhook verification, the opt-in context guard, read-only admin CLI tests, and authenticated health/readiness probes. Local source-level benchmarks measured the engine at 5.8–26.4 µs/op and context inspection at 1.40–1.61 µs/op on this environment; these exclude Bifrost HTTP, Redis, and provider latency and are not a gateway latency guarantee. Compose configuration, launcher shell syntax, and JSON files validated. The complete pinned Bifrost Docker image build and multi-container runtime were **not** run; the Compose file is a reproducible local harness, not evidence of a production deployment.

### Tracing and fault-injection checks (2026-11-10)

OpenTelemetry spans now cover plugin hooks, opt-in text inspection, risk scoring, and optional Redis product operations. The host must configure a tracer provider to export them; this change did not test an external exporter. Local chaos tests use miniredis and an in-process slow ledger to verify pass-through during optional-service failures. `go test ./...`, `go test -race ./...`, `go vet ./...`, the root Makefile build/chaos/demo/help targets, and `git diff --check` passed. No penalty box or automatic cooldown was added, and no real Redis Cluster, network-fault, pinned Compose, or production-traffic test was run.

### Custom rules, dashboard, and load-test guide (2026-11-10)

Custom RE2 prompt rules can be replaced atomically by trusted host code and add only a fixed advisory signal. The authenticated JSON dashboard exposes process-wide aggregates only, not active tenant counts or blocked-injection totals. The k6 script is loopback-first and its syntax check passed, but k6 is not installed and no load run was performed. `CONTRIBUTING.md` documents these boundaries and the test commands. Full gateway latency, live Redis/cluster resilience, and production behavior remain unverified.

### Limits of verification

No live-provider, production-traffic, multi-replica, or end-to-end gateway latency benchmark was run. No live Bifrost model adapter, payment or credential-status source, live company dashboard, shared behavioral-profile store, or automatic blocking is implemented. A pinned-source local overlay and a Compose smoke harness are included, but a full container build/runtime has not been verified. The separate company-facing page is synthetic only. Findings are heuristic indicators, not proof of fraud; false positives are possible. The patent-application reference below is author-provided: its legal status and whether its claims cover FraudGuard have not been independently verified.

See the [full verification report](VERIFICATION_REPORT.md) for detailed inspection notes and verification limits.

## Installation & Setup

### Run the module locally

The plugin module is in `plugins/fraudguard`. Use Go 1.27.3. Clone the public repository, then run:

```sh
git clone https://github.com/sham-cyber/Plugin.git
cd Plugin
cd plugins/fraudguard
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/demo
```

The demo uses synthetic requests, payment events, and trusted key-status flags. It does not contact an LLM provider or payment service. Go may download module dependencies the first time you run these commands.

From the repository root, `make help` lists convenient FraudGuard targets, including `make fraudguard-test`, `make fraudguard-chaos`, `make fraudguard-demo`, `make fraudguard-compose-demo`, and `make fraudguard-load-test`.

### Connect it to a Bifrost checkout

This is a companion module, not an upstream built-in Bifrost feature. For a local integration at the inspected Bifrost commit:

1. Set `BIFROST_ROOT` to a separate local checkout of commit `54ad3abec4fbbdaedf59414415094760fb9aa7e9`.
2. Run `bash plugins/fraudguard/integration/apply-bifrost-integration.sh --apply` to add the local module and built-in loader registration in that checkout.
3. Run the integration test as documented in the technical README. Review the resulting local diff.

This changes only the chosen local checkout; it does not commit or push to upstream. For an isolated Docker demonstration that prepares and removes its own pinned checkout, use `bash plugins/fraudguard/scripts/run-compose-demo.sh`. The full integration guide and limitations are in the [technical README](plugins/fraudguard/README.md).

## Implementation

- A Go adapter implements Bifrost's `schemas.LLMPlugin` hooks. It reads the authenticated internal virtual-key ID supplied by Bifrost; it does not use IP headers as identity.
- A bounded, process-local rolling profile compares model/provider use, request timing, token volume, and outcomes after warmup.
- Explainable signals produce a deterministic 0–100 score and threshold alert. The plugin is observe-only: it does not block or alter requests.
- Optional prompt checks inspect only user text transiently, use a bounded scan, and default off. Findings contain only fixed signal names and reasons, never prompt text.
- Optional trusted operator-managed RE2 rules can be replaced atomically in memory; matches produce a fixed advisory signal and are never written to Redis or used to block.
- Optional context inspection scans bounded user-role turns in the current request for staged override, sensitive-data disclosure, and schema-override phrases. It is heuristic, opt-in, transient, and never blocks.
- Findings are dispatched through a bounded background callback queue; queue drops and callback failures are counted in aggregate metrics.
- Optional payment-velocity/location and expired/revoked/exposed-key signals accept trusted host inputs. Bifrost does not provide those sources automatically, and no live source is connected.
- Optional threat indicators accept only trusted resolved internal key IDs, store HMAC digests with bounded TTL, and add a signal without blocking. An optional budget monitor reports threshold warnings from trusted host usage and pricing; it does not enforce caps or invent prices.
- HMAC-SHA256 webhook signature verification is available for receivers; it authenticates the body but does not prevent replay.
- Optional AI review is disabled by default. A Bifrost host may supply an isolated model adapter; it receives only the deterministic score and fixed signal summary, runs through a bounded background queue, and cannot change the score or alert. It interprets existing signals; it does not discover patterns outside the rule-based detector.
- The company-facing page is a visual prototype using synthetic AshaRatna Tech data. Its subscription and email/SMS alert previews are simulated; it is not connected to plugin findings and sends no messages.

## Technologies Used

- **Plugin:** Go 1.27.3 and `github.com/maximhq/bifrost/core` v1.14.1, implementing `schemas.LLMPlugin`.
- **Observability:** Prometheus-compatible metrics and OpenTelemetry metrics/traces with privacy-safe, bounded attributes.
- **Load testing:** Optional k6 script for the synthetic/local gateway; it targets loopback by default and measures end-to-end gateway behavior, not isolated plugin overhead.
- **Privacy and state:** Go standard library HMAC/SHA-256, bounded in-memory behavior profiles, optional Redis-backed usage/audit services, and an optional local Compose test harness. No provider-specific AI client is bundled; AI review requires an opt-in host adapter.
- **Company-facing prototype page:** React, TypeScript, and Vite; it displays synthetic demo data only.
- **Checks:** Go test, race detector, `go vet`, and the script for verifying against the pinned Bifrost source.
- **Operations:** read-only aggregate admin CLI and authenticated process-level JSON dashboard endpoint, plus local engine/context benchmarks. No tenant unblocking, raw Redis inspection, or policy mutation command is provided.

## Important scope

FraudGuard is a companion module, not a built-in upstream Bifrost feature. It implements Bifrost's LLM plugin interface and can be supplied to `core.Init`; an opt-in local overlay demonstrates the built-in HTTP-server registration against one pinned commit without changing the public upstream repository.

Behavioral profiles use bounded, in-memory state. Optional Redis services store product usage/audit data, not shared behavior profiles. FraudGuard does not retain prompts, infer identity from IP headers, use IP/user-agent threat fingerprints, or block requests. Context rules are simple phrases, not a guarantee against jailbreaks. Payment events, key status, threat-feed entries, and estimated pricing must be supplied by trusted host systems. Optional AI review is disabled by default; if enabled, the host supplies and owns the model client and provider credentials.

## Future Scope (Not Implemented)

The demo page is not a live company service: AshaRatna Tech is an illustrative label, the subscription state is simulated, and email/SMS alerts are previews only. The free hackathon tier and AshaRatna/Bifrost revenue routes are proposals in the documentation, not active commercial services. Nothing is sent, no real company account or subscription is created, and the page is not connected to live plugin findings.

For an actual deployment, the planned work is:

1. **Bifrost installation:** wire the module into a supported built-in server/plugin loader and document a repeatable install and configuration path. The host must also provide an isolated model adapter if AI review is enabled.
2. **Company access:** implement authentication, tenant separation, team permissions, and a real findings dashboard. Keep each company's profiles and findings private.
3. **Subscription and keys:** if the direct AshaRatna service is pursued, build company billing and a secure, separate FraudGuard entitlement model. If Bifrost is approached as a reseller, agree on distribution and billing terms first. The prototype neither issues product keys nor stores provider API keys; these must remain distinct concerns.
4. **Email and SMS:** connect verified destinations and a delivery provider, with consent, retry, and audit behavior. Current alert cards are simulated previews, not delivery.
5. **Trusted signals:** connect payment and key-status sources; deduplicate retried payment events with an idempotency ID. The plugin has optional input APIs, but no live sources.
6. **Production readiness:** design shared state and retention for multiple replicas, then measure latency, load, false positives, AI-adapter overhead, and operational failure modes.
7. **Advisory inputs:** connect a trusted threat-feed source and trusted pricing/usage source; current internal-key matching and budget monitoring are optional local helpers, not automated integrations or enforcement.
8. **Commercial validation:** test whether companies will pay for a managed workspace and what service level they need. The proposed direct-subscription and Bifrost-partner routes have no confirmed pricing, customers, or revenue forecast.

The Bifrost request path remains observe-only unless a separately reviewed future design changes that scope. Track trusted-source integration and retry deduplication in [GitHub Issue #1](https://github.com/sham-cyber/Plugin/issues/1).

## Authorship and repository proof

- **Project origin:** FraudGuard is a community hackathon prototype by [sham-cyber](https://github.com/sham-cyber), not an upstream Bifrost feature.
- **Git history:** [Commit `224d4eb`](https://github.com/sham-cyber/Plugin/commit/224d4ebc165e0db95ad9c88e3cb70ea58edca818), authored by `sham-cyber`, records the opt-in payment and key-status signal implementation. It is a feature commit, not the initial project commit.
- **License:** This repository's original code and documentation use Apache License 2.0, not MIT. Go source files carry matching copyright and SPDX notices. Bifrost and third-party components retain their own licenses.
- **Public record:** The GitHub repository is public and uses `main` as its default branch (checked 2026-11-09). Git history is available there.
- **Open scope:** [Issue #1](https://github.com/sham-cyber/Plugin/issues/1) remains open for trusted-source integration and retry deduplication. The local integration overlay only affects a separate checkout; no upstream installation, acceptance, or merge is claimed.

## Origin

FraudGuard adapts related behavioral anti-fraud ideas from Indian Patent Application No. 202521074434 (publication no. IN202521074434 A1), titled “AI-money maker: intelligent automated revenue generation system with anti-fraud transaction detection by AI.” The application describes a broader system; this module applies related behavior-scoring ideas to AI-gateway requests. This reference does not claim the application was granted or that its claims cover FraudGuard; status and claim coverage have not been independently verified.

## License

Unless otherwise noted, this repository's original project code and documentation are licensed under the Apache License, Version 2.0; see [LICENSE](LICENSE). Bifrost and third-party dependencies remain under their respective licenses.
