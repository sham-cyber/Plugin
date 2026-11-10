# FraudGuard Hackathon Presentation Outline

## Slide 1 — FraudGuard

**Behavior-based abuse signals for AI gateways**

An observe-only Go plugin module built around the Bifrost plugin contract. It reports behavioral changes while leaving requests unchanged.

## Slide 2 — Request counts can miss behavior changes

- Fixed request limits measure volume, not a key's changing behavior.
- A key can stay below a limit while probing new models/providers, changing token volume, or generating repeated errors.
- The inspected Bifrost commit (`54ad3abec4fbbdaedf59414415094760fb9aa7e9`) had no dedicated per-key behavioral fraud plugin.

## Slide 3 — Build a baseline, explain deviations

- Bounded per-key history covers provider/model mix, token volume, request spacing, and failures.
- Deterministic 0–100 scores include named reasons and configurable warmup/window behavior.
- Prompt and multi-turn checks are opt-in, bounded, transient, and never retained.
- Findings are advisory. FraudGuard does not block, throttle, or rewrite requests.

## Slide 4 — Demonstrate the evidence

Run from `plugins/fraudguard`:

```sh
GOSUMDB=sum.golang.org go test -race ./...
GOSUMDB=sum.golang.org go run ./cmd/demo
```

The local test suite covers the Bifrost core plugin pipeline, synthetic Redis fixtures, privacy, concurrency, usage metering, and observe-only outcomes. No live provider or payment request is made by the demo.

## Slide 5 — Product model and honest boundaries

- The Apache-2.0 plugin core is free to inspect; AshaRatna Tech could offer company-facing reporting, integrations, and support.
- Bifrost can supply tenant-scoped usage aggregates; AshaRatna owns its commercial rate card and financial reporting.
- The dashboard's current cost figures and findings are synthetic. A live AshaRatna API and company login are not mounted.
- FraudGuard is not a built-in Bifrost feature and has not been accepted or merged upstream.
- Production deployment, live Redis topology, multi-replica behavior, real invoices, and latency under gateway traffic remain unverified.

## Suggested close

“FraudGuard adds explainable, privacy-conscious behavior signals to the gateway path without taking control of request handling. The prototype proves the plugin contract and testable scoring flow; a company-facing live service still needs authenticated data sources and deployment validation.”
