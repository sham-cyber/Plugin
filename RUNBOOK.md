# FraudGuard Operations Runbook

## Status and operating limits

FraudGuard is a production-oriented companion module, not a built-in Bifrost feature and not production-validated. A host must load it through its supported plugin configuration and provide any private health/admin listener. This runbook is not a claim that the project is ready for an unreviewed production rollout.

FraudGuard is observe-only:

- It reports behavioral findings and never blocks, throttles, or rewrites inference requests or payments.
- Behavioral profiles are bounded and process-local. They reset on restart and are not shared between gateway replicas.
- Redis is optional and is used only for product usage, policy, audit, and webhook services. It is not a behavioral profile store or request limiter.
- The host/Bifrost authenticates callers and supplies resolved internal identity. FraudGuard does not parse Authorization JWTs or trust caller-supplied tenant claims, IP headers, or user-agent strings.
- Prompt checks and custom rules are opt-in, bounded, transient, and advisory. Prompts and rule text are not retained in findings.

## Developer setup and local validation

From the module directory:

```sh
bash scripts/setup_dev.sh
```

This checks Go and runs the build, tests, race detector, vet, and synthetic demo. It does not create `.env` files or pull containers. Go may download module dependencies according to `go.mod` and the configured checksum database.

To run the pinned Bifrost/Redis/mock-provider HTTP smoke test, use the explicit network/container option:

```sh
bash scripts/setup_dev.sh --compose-demo
```

This fetches the pinned Bifrost source and images, starts a disposable local stack, sends synthetic requests, and removes the temporary checkout and containers on exit. It requires Git, Docker Compose, and network access. The default setup path does not run it.

The existing `main_test.go` test sends a request through Bifrost's core plugin pipeline to an ephemeral HTTP mock provider. It verifies an alerting prompt finding still reaches the provider. It does not start Bifrost's inbound HTTP gateway; the Compose smoke test covers that separate path when run.

## Build and integration

Build metadata is supplied at link time:

```sh
VERSION=v1.2.3 \
COMMIT="$(git rev-parse --short=12 HEAD)" \
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  make build
```

When the plugin is built as part of the Bifrost host, pass equivalent `-X` linker values for:

- `github.com/maximhq/bifrost/plugins/fraudguard.BuildVersion`
- `github.com/maximhq/bifrost/plugins/fraudguard.BuildCommit`
- `github.com/maximhq/bifrost/plugins/fraudguard.BuildTime`

Without a semantic version tag or supplied timestamp, values fall back to `dev` and `unknown`. Release metadata appears in health/admin JSON, not metric labels.

The module implements Bifrost's plugin contract but is not auto-loaded by upstream. Use the documented compatibility commit and a host-supported registration path. Test any integration overlay only against a disposable checkout; do not apply it automatically to a user's separate Bifrost tree.

## Configuration and secrets

Keep provider credentials in Bifrost's own secret mechanism. Keep FraudGuard service credentials in the deployment secret manager; do not commit them in `.env`, JSON, shell history, or logs.

Optional product Redis setup requires all of:

- `FRAUDGUARD_REDIS_URL`
- `FRAUDGUARD_TENANT_HMAC_KEY`
- `FRAUDGUARD_TENANT_ID` as a trusted single-tenant fallback

For a multi-tenant host, resolve the tenant from authenticated server-side context instead of a caller header. Optional webhook configuration requires `FRAUDGUARD_WEBHOOK_URL`, `FRAUDGUARD_WEBHOOK_SIGNING_KEY`, and `FRAUDGUARD_WEBHOOK_ALLOWED_HOSTS`. See the module README for exact requirements and supported detector overrides. Do not print or paste secret values into diagnostics.

The request path remains fail-open if optional Redis product services are unavailable. Metering, plan, audit, or webhook data can be delayed or incomplete during an outage; inference continues. Do not treat the optional usage ledger as the sole financial source of truth.

## Redis, caching, and replica behavior

The in-memory tier-policy cache is TTL-based and LRU-bounded. `ProductServices.MaxCachedPolicies` defaults to 10,000 and may be configured up to 100,000. Policy TTL defaults to 30 seconds and cannot exceed 10 minutes. Cache keys are digests rather than raw tenant IDs. Custom prompt rules are compiled and atomically swapped, so they do not need a per-request lookup cache.

There is no IP blocklist, Redis Lua rate limiter, or distributed behavioral history. The current Redis integration has not been validated against Redis Cluster; do not assume cluster support or shared detector state. Select a Redis topology only for the optional product services after validating it with the host's Redis client and operational requirements.

## Monitoring and alert response

Mount health and admin handlers only on a private listener and provide host-owned authorization. Relevant routes include `/healthz`, `/readyz`, `/admin/fraudguard/status`, `/admin/fraudguard/dashboard`, and `/admin/fraudguard/metrics`.

Recommended initial operator alerts:

- Health reports not-ready or degraded status.
- Finding-callback or product-event drops become nonzero or continue increasing.
- Redis circuit remains open, or optional usage/audit/webhook failures rise.
- Policy cache misses increase unexpectedly after tenant policies are warmed.
- Host-measured request latency breaches the host's own SLO.

FraudGuard's default risk alert threshold is 60. Tune it using representative, consented staging traffic and review false positives before enabling additional prompt, payment, key-status, cross-key, or AI-review signals. Scores and plan overages are advisory, not enforcement. The module's detector benchmarks do not establish gateway latency or a production SLO.

`RunSecurityChecks` and the security section of health/admin JSON provide configuration bounds and fixed architecture status codes only. The report explicitly says secret values were not inspected; it is not a dependency vulnerability scan or security certification. Run the organization's approved dependency and deployment security checks separately.

## Troubleshooting

| Symptom | Checks and response |
| --- | --- |
| No behavioral profiles or findings | Confirm Bifrost supplies its resolved internal virtual-key ID to the plugin. Allow the configured warmup period; profiles are process-local. |
| Prompt signal is absent | Confirm the prompt feature is enabled, the plan permits it, and the input is user text within configured scan bounds. The feature is heuristic and may not match every attack. |
| Policy-only signals are missing | Confirm the trusted tenant resolver returns the correct tenant and warm the policy through the host's trusted `ResolvePolicy` path. A cache miss does not block inference. |
| Redis circuit is open | Check Redis connectivity and host secret-manager configuration without printing URLs or credentials. Inference remains available; expect product data gaps until recovery. |
| Admin/health endpoint returns unauthorized | Confirm the host authorizer and private-listener routing. Do not expose these endpoints publicly to bypass the host's auth layer. |
| Compose smoke test fails | Check Docker/Compose, network access to the pinned source/images, and the printed container logs. The script cleans its disposable stack on exit. |

## Rollout and rollback

1. Run `bash scripts/setup_dev.sh` and record the exact Go/Bifrost versions.
2. In staging, run the pinned HTTP smoke test and the host's own integration checks. Test benign behavior, alert pass-through, Redis outage pass-through, and monitoring authorization.
3. Begin with prompt and optional product signals disabled unless the host has a trusted source and an approved reason to enable them.
4. Observe findings and tune thresholds. Do not treat findings as automatic fraud decisions.
5. Before production, independently verify multi-replica state expectations, Redis topology, identity provisioning, privacy/retention, dependency vulnerabilities, latency, alert delivery, and recovery procedures.
6. If behavior is unexpected, disable optional signals or remove the plugin registration through the host's supported configuration and return to the last known-good gateway build. No request blocklist or penalty-box cleanup is required because this module does not block.
