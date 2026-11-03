# FraudGuard: Bifrost behavior-detection prototype

This project contains an experimental, observe-only Go plugin prototype for the Bifrost AI gateway. It looks for changes in request behavior that fixed request-count limits can miss. It reports findings; it never blocks requests.

## Project documents

The technical README includes setup, demo, tests, Bifrost integration, privacy limits, and limitations:

[Open the full FraudGuard README](plugins/fraudguard/README.md)

[Open the separate presentation guide](PRESENTATION_GUIDE.md)  
[Open the verification report](VERIFICATION_REPORT.md)

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
