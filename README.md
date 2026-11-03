# FraudGuard: Bifrost behavior-detection prototype

This project contains an experimental, observe-only Go plugin prototype for the Bifrost AI gateway. It looks for changes in request behavior that fixed request-count limits can miss. It reports findings; it never blocks requests.

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

The prototype maintains bounded, in-memory, per-virtual-key behavior profiles and produces deterministic 0–100 scores with named signals. It covers model/provider novelty, rapid request sequences, token-volume spikes, and repeated failures. It is observe-only: it never blocks or changes a request. Prompts and payloads are not retained; cross-key correlation is opt-in and does not use IP addresses or request headers. It has no Redis, external AI service, or network dependency.

### Checks run

These checks were run on 2026-11-03 with Go 1.27.3:

- `GOWORK=/tmp/fraudguard-validation.work GOSUMDB=sum.golang.org go test ./...` — passed using a temporary Go workspace linked to the pinned local Bifrost `core` source.
- `GOSUMDB=sum.golang.org go test -race ./...` — passed.
- `GOSUMDB=sum.golang.org go vet ./...` — passed with no diagnostics.
- `GOSUMDB=sum.golang.org go run ./cmd/demo` — passed; output included `score=30`, `alert=false`, and `signals=[rapid_sequence]`. The demo makes no network calls.

The tests cover benign baselines, anomalies below fixed rate limits, warmup, deterministic scoring, token spikes, repeated failures, concurrency, state bounds and expiration, privacy, exemptions, threshold behavior, and observe-only hooks.

### Limits of verification

No live-provider, production-traffic, multi-replica, or latency benchmark was run. No dashboard, shared store, automatic blocking, or built-in HTTP-server registration is implemented. Findings are heuristic indicators, not proof of fraud; false positives are possible. The patent-application reference below is author-provided: its legal status and whether its claims cover FraudGuard have not been independently verified.

See the [full verification report](VERIFICATION_REPORT.md) for detailed inspection notes and verification limits.

## Quick start

```sh
cd plugins/fraudguard
go test ./...
go test -race ./...
go run ./cmd/demo
```

The demo uses synthetic requests only. It does not contact an LLM provider.

## Important scope

FraudGuard is a companion module, not a built-in Bifrost HTTP-server plugin. It implements Bifrost's LLM plugin interface and can be supplied to `core.Init`; the full guide explains what separate upstream wiring would be needed to make it a built-in feature.

It uses bounded, in-memory state. It does not use Redis, retain prompts, infer identity from IP headers, or block requests. No upstream installation, acceptance, or merge is claimed.

## Origin

FraudGuard adapts related behavioral anti-fraud ideas from Indian Patent Application No. 202521074434 (publication no. IN202521074434 A1), titled “AI-money maker: intelligent automated revenue generation system with anti-fraud transaction detection by AI.” The application describes a broader system; FraudGuard is an experimental Bifrost plugin applying related behavior-scoring ideas to AI-gateway requests. This reference does not claim the application was granted or that its claims cover FraudGuard; status and claim coverage have not been independently verified.

## License

Unless otherwise noted, this repository's original project code and documentation are licensed under the Apache License, Version 2.0; see [LICENSE](LICENSE). Bifrost and third-party dependencies remain under their respective licenses.
