# FraudGuard: Bifrost behavior-detection prototype

This project contains an experimental, observe-only Go plugin prototype for the Bifrost AI gateway. It looks for request-behavior changes that fixed request-count limits can miss, with separate opt-in scorers for trusted payment events and credential-status inputs. It reports findings; it never blocks requests or payments.

## Project documents

The technical README includes setup, demo, tests, Bifrost integration, privacy limits, and limitations:

[Open the full FraudGuard README](plugins/fraudguard/README.md)

[Open the verification report](VERIFICATION_REPORT.md)

## Verification Report — What Was Checked and What It Proves

This section is the project's evidence summary. It distinguishes what was checked from what remains unverified.

### Upstream inspection

The Bifrost source was inspected at commit [`54ad3abec4fbbdaedf59414415094760fb9aa7e9`](https://github.com/maximhq/bifrost/tree/54ad3abec4fbbdaedf59414415094760fb9aa7e9). The inspected plugin list has no `fraudguard` module, and the inspected `plugins/` and `core/` Go source contain no dedicated per-virtual-key behavior-based fraud detector. Bifrost Governance provides configured request/token limits; core key rotation is tied to provider failures and retries. This finding is limited to the pinned source inspected.

### Plugin compatibility and scope

The prototype implements Bifrost's `schemas.LLMPlugin` hooks and includes a compile-time interface assertion and hook tests. It can be supplied through `core.Init` using `schemas.BifrostConfig.LLMPlugins`. It is **not** connected to Bifrost's built-in HTTP-server plugin loader, and it has not been installed, accepted, or merged upstream. No upstream source files were changed.

The prototype maintains bounded, in-memory, per-virtual-key behavior profiles and produces deterministic 0–100 scores with named signals. It covers model/provider novelty, rapid request sequences, token-volume spikes, and repeated failures. Separate opt-in scorers are also available for trusted payment-event velocity/location and trusted expired/revoked/exposed key-status inputs. Those inputs are not supplied automatically by Bifrost and all findings remain observe-only. Prompts and payloads are not retained; cross-key correlation is opt-in and does not use IP addresses or request headers. Optional AI review is also available through a host-supplied Bifrost model adapter; it receives only a sanitized finding summary and cannot change the deterministic score or request handling.

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

### Limits of verification

No live-provider, production-traffic, multi-replica, or latency benchmark was run. No live Bifrost model adapter, payment or credential-status source, live company dashboard, shared store, automatic blocking, or built-in HTTP-server registration is implemented. The separate company-facing page is synthetic only. Findings are heuristic indicators, not proof of fraud; false positives are possible. The patent-application reference below is author-provided: its legal status and whether its claims cover FraudGuard have not been independently verified.

See the [full verification report](VERIFICATION_REPORT.md) for detailed inspection notes and verification limits.

## Installation & Setup

### Run the local prototype

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

### Connect it to a Bifrost checkout

This is a companion module, not an installable built-in Bifrost feature. For a manual integration at the inspected Bifrost commit:

1. Copy `plugins/fraudguard` into the matching plugin directory in that Bifrost checkout.
2. Add the module to the app/server Go workspace or module graph.
3. Create `fraudguard.New(...)` and pass it through `schemas.BifrostConfig.LLMPlugins` to `core.Init`.

This direct `core.Init` path is not wired into Bifrost's built-in HTTP plugin loader. The full integration example and limitations are in the [technical README](plugins/fraudguard/README.md). No upstream Bifrost files were modified.

## Implementation

- A Go adapter implements Bifrost's `schemas.LLMPlugin` hooks. It reads the authenticated internal virtual-key ID supplied by Bifrost; it does not use IP headers as identity.
- A bounded, process-local rolling profile compares model/provider use, request timing, token volume, and outcomes after warmup.
- Explainable signals produce a deterministic 0–100 score and threshold alert. The plugin is observe-only: it does not block or alter requests.
- Optional payment-velocity/location and expired/revoked/exposed-key signals accept trusted host inputs. Bifrost does not provide those sources automatically, and no live source is connected.
- Optional AI review is disabled by default. A Bifrost host may supply an isolated model adapter; it receives only the deterministic score and fixed signal summary, runs through a bounded background queue, and cannot change the score or alert. It interprets existing signals; it does not discover patterns outside the rule-based detector.
- The company-facing page is a visual prototype using synthetic AshaRatna Tech data. Its subscription and email/SMS alert previews are simulated; it is not connected to plugin findings and sends no messages.

## Technologies Used

- **Plugin:** Go 1.27.3 and `github.com/maximhq/bifrost/core` v1.14.1, implementing `schemas.LLMPlugin`.
- **Privacy and state:** Go standard library HMAC/SHA-256 and bounded in-memory state; no Redis or provider-specific AI client is bundled. AI review requires an opt-in host adapter.
- **Company-facing prototype page:** React, TypeScript, and Vite; it displays synthetic demo data only.
- **Checks:** Go test, race detector, `go vet`, and the script for verifying against the pinned Bifrost source.

## Important scope

FraudGuard is a companion module, not a built-in Bifrost HTTP-server plugin. It implements Bifrost's LLM plugin interface and can be supplied to `core.Init`; the full guide explains what separate upstream wiring would be needed to make it a built-in feature.

It uses bounded, in-memory state. It does not use Redis, retain prompts, infer identity from IP headers, or block requests. Payment events and key status are not fetched automatically; the host must supply them from trusted systems. Optional AI review is disabled by default; if enabled, the host supplies and owns the model client and provider credentials.

## Future Scope (Not Implemented)

The demo page is not a live company service: AshaRatna Tech is an illustrative label, the subscription state is simulated, and email/SMS alerts are previews only. Nothing is sent, no real company account or subscription is created, and the page is not connected to live plugin findings.

For an actual deployment, the planned work is:

1. **Bifrost installation:** wire the module into a supported built-in server/plugin loader and document a repeatable install and configuration path. The host must also provide an isolated model adapter if AI review is enabled.
2. **Company access:** implement authentication, tenant separation, team permissions, and a real findings dashboard. Keep each company's profiles and findings private.
3. **Subscription and keys:** define subscription management and a secure product/API-key issuance model. This prototype neither issues product keys nor stores provider API keys; those must remain distinct concerns.
4. **Email and SMS:** connect verified destinations and a delivery provider, with consent, retry, and audit behavior. Current alert cards are simulated previews, not delivery.
5. **Trusted signals:** connect payment and key-status sources; deduplicate retried payment events with an idempotency ID. The plugin has optional input APIs, but no live sources.
6. **Production readiness:** design shared state and retention for multiple replicas, then measure latency, load, false positives, AI-adapter overhead, and operational failure modes.

The Bifrost request path remains observe-only unless a separately reviewed future design changes that scope. Track trusted-source integration and retry deduplication in [GitHub Issue #1](https://github.com/sham-cyber/Plugin/issues/1).

## Authorship and repository proof

- **Project origin:** FraudGuard is a community hackathon prototype by [sham-cyber](https://github.com/sham-cyber), not an upstream Bifrost feature.
- **Git history:** [Commit `224d4eb`](https://github.com/sham-cyber/Plugin/commit/224d4ebc165e0db95ad9c88e3cb70ea58edca818), authored by `sham-cyber`, records the opt-in payment and key-status signal implementation. It is a feature commit, not the initial project commit.
- **License:** This repository's original code and documentation use Apache License 2.0, not MIT. Go source files carry matching copyright and SPDX notices. Bifrost and third-party components retain their own licenses.
- **Public record:** The GitHub repository is public and uses `main` as its default branch (checked 2026-11-09). Git history is available there.
- **Open scope:** [Issue #1](https://github.com/sham-cyber/Plugin/issues/1) remains open for trusted-source integration and retry deduplication. No Bifrost upstream files were changed, and no upstream installation, acceptance, or merge is claimed.

## Origin

FraudGuard adapts related behavioral anti-fraud ideas from Indian Patent Application No. 202521074434 (publication no. IN202521074434 A1), titled “AI-money maker: intelligent automated revenue generation system with anti-fraud transaction detection by AI.” The application describes a broader system; FraudGuard is an experimental Bifrost plugin applying related behavior-scoring ideas to AI-gateway requests. This reference does not claim the application was granted or that its claims cover FraudGuard; status and claim coverage have not been independently verified.

## License

Unless otherwise noted, this repository's original project code and documentation are licensed under the Apache License, Version 2.0; see [LICENSE](LICENSE). Bifrost and third-party dependencies remain under their respective licenses.
