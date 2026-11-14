# FraudGuard — Hackathon Submission

## Summary

FraudGuard is an observe-only Go plugin module for Bifrost. It looks for changes in how an authenticated virtual key uses AI models—such as model/provider probing, unusual token volume, rapid request sequences, or repeated failures—and reports explainable risk findings. It does not block or modify requests.

The hackathon problem is a specific feature gap, not an absence of a Bifrost plugin system: at the inspected commit, Bifrost had governance limits, retry-related provider key rotation, and a shared-object loader, but no dedicated per-virtual-key behavior detector. FraudGuard is the proposed community plugin for that missing detector.

The code is in `plugins/fraudguard`. It implements Bifrost's `schemas.LLMPlugin` contract and has a native `.so` adapter for Bifrost's existing loader. The optional local integration overlay targets Bifrost commit `54ad3abec4fbbdaedf59414415094760fb9aa7e9` (2026-11-03). FraudGuard itself is not a built-in Bifrost feature and has not been accepted or merged upstream.

## Architecture

```text
Bifrost request hook
  ├─ resolve trusted internal virtual-key identity
  ├─ inspect enabled signals using bounded, transient inputs
  ├─ compare request metadata with a bounded in-memory profile
  └─ produce a deterministic 0–100 advisory finding
       ├─ aggregate metrics / bounded finding callback
       └─ optional background usage, audit, and webhook services
```

- Profiles are process-local, bounded, and expire after inactivity. Redis is not the behavioral profile store.
- The native `.so` adapter uses Bifrost's existing shared-object loader; it does not add a new loader feature to Bifrost.
- Findings contain named signals and aggregate metadata, not prompt text, payloads, bearer keys, or raw key IDs.
- Prompt and multi-turn context checks are disabled by default. When enabled, they scan bounded user text transiently and return only fixed signal flags.
- Payment velocity/location and credential-status checks require trusted host-provided inputs. There is no payment-provider or credential registry connection in this submission.
- Redis is optional for product metering/audit services. Health probes can ping the configured store, but an optional Redis outage does not block inference.
- The health/readiness handler requires host authorization and private mounting. It is not automatically wired into Bifrost's public HTTP routes.

## Detection boundaries and safety

The detector is heuristic and can raise false positives. It is not a guarantee against fraud, prompt injection, credential theft, or payment abuse. It does not inspect IP headers or user-agent values as identity, does not correlate keys using network fingerprints, and does not store raw prompts. It does not block requests, payments, or tenants.

Step 29's proposed IP/tenant blocklist API is intentionally not implemented: IP headers are not a trusted identity and automatic blocking conflicts with the observe-only MVP. Existing exemptions use trusted, resolved internal virtual-key IDs through controlled runtime configuration; there is no public policy-management API or Redis-backed block/allow list.

## Setup and judge walkthrough

Requirements: Go 1.27.3, Git, and (for the container demo) Docker with Compose v2 and network access to fetch the pinned Bifrost source and images.

From the repository root:

```sh
cd plugins/fraudguard
make verify
make bench
cd ../..

# Unit tests and synthetic findings, without Docker
bash plugins/fraudguard/scripts/demo_walkthrough.sh --local

# Optional local Bifrost + mock-provider smoke test
bash plugins/fraudguard/scripts/demo_walkthrough.sh --compose
```

The local demo prints deterministic synthetic examples, including an alerting multi-turn context case. The Compose smoke path sends benign and anomalous synthetic requests through the pinned local Bifrost build and expects both to receive HTTP 200. It then prints separate in-process synthetic detector results. Those scores are not presented as live gateway findings. The local mock provider does not call an external model.

The Compose mode was run successfully on 2026-11-14 against the pinned Bifrost commit. Baseline and synthetic anomalous requests both received HTTP 200; the anomaly produced a sanitized alert and remained unchanged. This is a local synthetic smoke test, not evidence of a production deployment. The authenticated admin CLI can read `/admin/fraudguard/metrics`, `/status`, and `/healthz` only when the host mounts the handler and provides authorization; no such endpoint is mounted by default.

## Health and readiness

Hosts may mount `NewProbeHandler` on a private listener with an authorization callback and connect `/livez`, `/healthz`, and `/readyz` to their probe system. The handler reports process heap allocation (not plugin-only memory), readiness, and Redis status without returning backend error text. Redis is optional for inference readiness by default. A host can configure a heap threshold and require Redis for readiness if its own deployment depends on Redis-backed product services. No probe is automatically registered in the Bifrost HTTP server.

## Performance measurements

The checked-in benchmark target is `make bench`. On the Replit Linux x86-64 environment with an Intel Xeon Platinum 8581C, a one-second benchmark run measured:

| Benchmark | `-cpu 1` | `-cpu 4` |
|---|---:|---:|
| Engine start/complete, sequential | 16.8 µs/op | 26.4 µs/op |
| Engine start/complete, parallel | 5.8 µs/op | 10.3 µs/op |
| Context inspection | 1.40 µs/op | 1.61 µs/op |

These are local in-process measurements. They exclude Bifrost HTTP handling, Redis, provider/network latency, and deployment scheduling. They do not establish a sub-5-ms end-to-end service-level objective.

## Free hackathon edition and possible revenue

### Free version for the hackathon

Keep the current Apache-2.0 plugin, test suite, synthetic demo, and setup instructions free to use for the hackathon. There is no checkout, subscription, or FraudGuard license key in this build. Customers continue to use their own model-provider API keys in Bifrost; those keys are not FraudGuard licenses and must never be collected by this page.

### Route A: AshaRatna Tech sells a service directly

AshaRatna could sell a managed FraudGuard service to companies that want a company-facing dashboard, multi-instance reporting, alert delivery, longer audit retention, setup help, and ongoing support. A practical pricing hypothesis is a monthly or annual subscription for a company workspace, with tiers based on the service level and support—not a fee every time the software is deployed or a charge for each model-provider API key.

This paid service is proposed, not built. The project does not yet have company authentication, hosted multi-company data, production alert delivery, a subscription system, license verification, or a payment connection. Any price should be set after pilot discussions and a real cost review. No customer or revenue is assumed.

### Route B: Bifrost distributes it as a partner

If Bifrost approves the integration and agrees to commercial terms, it could list, bundle, or resell a paid FraudGuard service. AshaRatna and Bifrost could then agree on a revenue share or licensing arrangement. This needs Bifrost's written agreement, technical review, and an agreed way to handle billing; no Bifrost partnership, marketplace listing, or revenue share exists today.

### How revenue and profit would be measured

- Direct route: revenue is the number of paying company workspaces multiplied by the agreed subscription price.
- Partner route: revenue is the agreed share of paid sales after the parties define billing and settlement.
- Net profit is revenue minus measured hosting, delivery, support, compliance, and other operating costs. No profit figure is promised because pricing, customers, and costs have not been validated.

Because the plugin code is Apache-2.0, the commercial value should come from optional managed services, operational features, support, or separately licensed additions—not a claim that users cannot run the open-source code. A future FraudGuard entitlement key would be separate from every model-provider API key and is not implemented in the hackathon build. Usage exports and tier-policy helpers exist, but the prototype does not create invoices, charge customers, or enforce hard spend caps.

## CI and verification

GitHub Actions remain pending and were not run. A CI workflow draft is present on the current local branch but has not been pushed to GitHub or executed.

Locally verified for this submission:

- `make verify`: build, tests, race detector, vet, and synthetic demo passed.
- `make bench`: completed and emitted the measurements above.
- `demo_walkthrough.sh --local`: completed.
- Pinned Bifrost `.so` build and loader checks passed; the server package test passed.
- Full local Compose smoke passed on 2026-11-14 with Redis TCP readiness, mock-provider HTTP readiness, and observe-only HTTP 200 pass-through for baseline and anomalous requests.
- Compose configuration, shell syntax, JSON config, and `git diff --check` passed.

## Technologies

Go 1.27.3, Bifrost core plugin interfaces and shared-object loader, Redis client (optional product services), OpenTelemetry metrics, Prometheus text metrics, miniredis test fixtures, and Docker Compose. GitHub Actions are pending and were not run.
