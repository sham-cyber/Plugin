# Contributing to FraudGuard

FraudGuard is an experimental companion Go module for Bifrost. Contributions should improve explainable, privacy-conscious detection while preserving the current observe-only behavior. It is not a built-in upstream Bifrost feature, and this repository does not claim upstream acceptance.

## Before proposing a change

- Keep findings advisory. Do not add request, payment, tenant, or IP blocking, throttling, penalty boxes, or automatic cooldowns.
- Never log or retain raw prompts, request bodies, bearer keys, provider credentials, or tenant identifiers in findings, traces, metrics, or demo output.
- Attribute a request only from Bifrost's resolved internal virtual-key context or a trusted host callback. Do not treat IP headers, user-agent strings, JWT claims supplied by the request, or caller-controlled tenant headers as identity.
- Keep prompt and context inspection opt-in, bounded, and transient. New heuristic patterns must use bounded input and include false-positive tests.
- Use only trusted host sources for payment, credential-status, and tenant data. Redis is for optional product metering/audit services, not behavioral profiles or remotely supplied rules.
- Do not claim production performance, fraud prevention, or upstream compatibility beyond the tests actually run.

## Adding an advisory prompt rule

Use `CustomPromptRules` for trusted operator-managed RE2 expressions. Rule documents are bounded by rule count, pattern size, and JSON size; invalid replacements leave the previous rule set active. A match produces only the fixed `custom_prompt_pattern` signal. Rule text and prompt text are not included in the finding.

Updates are performed by trusted host code through `ReplaceJSON`; the module provides no Redis-backed rule writer or unauthenticated update endpoint. Callers are responsible for authenticating and authorizing the source of rule updates.

## Local checks

From the repository root:

```sh
make fraudguard-build
make fraudguard-test
make fraudguard-race
make fraudguard-vet
make fraudguard-chaos
make fraudguard-demo
```

The chaos tests use miniredis and in-process fakes. They do not prove behavior under production Redis Cluster, real network faults, or production traffic.

The optional `make fraudguard-load-test` requires k6 and a running local Bifrost endpoint backed by the synthetic mock provider. It targets loopback by default, sends synthetic requests, and reports whole-gateway results—not isolated plugin latency. Non-local targets are refused unless explicitly allowed; only use that override for systems you own or are authorized to test.

The Docker Compose walkthrough (`make fraudguard-compose-demo`) requires Docker and network access to fetch the pinned Bifrost source and images. It uses a disposable checkout and synthetic credentials.

## Pull requests

1. Keep changes focused and add tests for both detection and benign cases.
2. Add privacy assertions when a new field or signal is introduced.
3. Run the checks relevant to the change and state clearly which checks were not run.
4. Include the Bifrost version or commit used for compatibility checks; do not imply the plugin is installed upstream.
5. Keep the Apache-2.0 license headers and do not include secrets or real customer traffic in fixtures.
