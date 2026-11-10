# FraudGuard: Bifrost observe-only behavior plugin

FraudGuard is an observe-only Go plugin module for the Bifrost AI gateway. It scores behavioral changes that request-count limits can miss, such as a sudden provider/model probe, a sharp usage-volume change, unusually fast requests relative to a key's own history, or a burst of failures. Redis-backed tenant metering, tier policies, sanitized audit records, and signed webhooks are optional product services. This is not yet validated as a live production deployment: detector profiles remain process-local, external trusted sources are not connected, and production workload checks have not been run.

This code is a companion module in this Replit project. A local integration overlay and installer are included for the pinned Bifrost commit, but they do not change the upstream repository by themselves. FraudGuard is **not** included in upstream Bifrost, and no upstream acceptance or merge is claimed.

## Upstream contract and compatibility

The hook shape was inspected at upstream commit [`54ad3abec4fbbdaedf59414415094760fb9aa7e9`](https://github.com/maximhq/bifrost/tree/54ad3abec4fbbdaedf59414415094760fb9aa7e9), specifically `core/schemas/plugin.go` and `plugins/governance/main.go`.

At that revision, Bifrost has governance budgets/rate limits and provider-failure retry key rotation, but no dedicated behavior-based fraud detector. FraudGuard targets that specific gap; it complements rather than replaces Governance.

At that commit, an LLM plugin implements `schemas.LLMPlugin`:

- `GetName() string`
- `Cleanup() error`
- `PreRequestHook(ctx, req) error`
- `PreLLMHook(ctx, req) (*BifrostRequest, *LLMPluginShortCircuit, error)`
- `PostLLMHook(ctx, response, bifrostErr) (*BifrostResponse, *BifrostError, error)`

The standalone module pins `github.com/maximhq/bifrost/core v1.14.1`, the same core module version used by upstream sibling plugins at the inspected commit. The compile-time interface assertion and plugin hook test check the adapter contract when run against that core API.

## Technologies Used
For deployment prerequisites, health monitoring, Redis boundaries, troubleshooting, and rollout/rollback guidance, see the repository [operations runbook](../../RUNBOOK.md).

For the proposed pricing structure and the limits of the current billing export, see the repository [monetization architecture](../../MONETIZATION.md).


- **Go:** version 1.27.3 for the plugin module.
- **Bifrost:** `github.com/maximhq/bifrost/core` v1.14.1 and the `schemas.LLMPlugin` contract.
- **State and product services:** bounded Go profiles plus optional Redis-backed tenant usage and audit storage; Redis identities use HMAC-derived references.
- **Webhook delivery:** standard-library HTTPS client with HMAC signatures, host allowlisting, bounded timeouts, and no redirects.
- **Observability:** Prometheus text exposition, optional OpenTelemetry API metrics, and privacy-safe OpenTelemetry tracing.
- **Release metadata:** Validated semantic version, short Git commit, and optional RFC3339 build time in authenticated health/admin JSON; no build labels are added to metric series.
- **Tests:** Go tests, race detector, vet, miniredis fixtures, a pinned-source Bifrost integration test, and an optional local k6 script.
- **Optional AI review:** a host-supplied adapter using the Bifrost host's selected model; no provider-specific model client or API key is bundled in this module.
- **Verification:** `go test`, `go test -race`, `go vet`, and a script that can test against the pinned Bifrost source checkout.

The plugin contains no provider-specific AI client. With AI review disabled it makes no AI model calls; when enabled, the host-supplied adapter owns the Bifrost model call. Redis and the signed webhook are contacted only when explicitly configured. No payment processor, Stripe billing service, or credential registry is connected. Phrase and multi-turn context checks are optional and default off; when enabled, only bounded user text in the current request is inspected transiently, and no prompt text is retained or emitted. Go may access the module proxy to download dependencies during setup.

## Related research context

FraudGuard adapts related behavioral anti-fraud ideas from Indian Patent Application No. 202521074434 (publication no. IN202521074434 A1), titled “AI-money maker: intelligent automated revenue generation system with anti-fraud transaction detection by AI.” The application describes a broader system; this module applies related behavior-scoring ideas to AI-gateway requests. This reference does not claim the application was granted or that its claims cover FraudGuard; status and claim coverage have not been independently verified.

## Threat model

FraudGuard is intended to help operators investigate abuse of an authenticated Bifrost virtual key when each individual request remains below a fixed rate limit. It looks for shifts relative to that key's recent history, not proof of malicious intent.

### Detects

- A provider or model not observed for that key during its retained rolling window.
- Request timing that is substantially faster than the key's own recent typical interval.
- Large changes in completed input/output token volume.
- Repeated failures that dominate recent outcomes, including malformed or denied requests when those outcomes reach the plugin's post-hook.
- Optional phrase-based user-text checks for common override/jailbreak wording, when explicitly enabled.
- Optional, trusted operator-managed RE2 prompt patterns; matches add one fixed advisory signal and never block.
- Optional prompt-size signal when user text exceeds the configured byte threshold.
- Exemptions for resolved internal virtual-key IDs, applied before profile creation.
- Excessive distinct virtual keys associated with a trusted authenticated user identity, when explicitly enabled.
- Payment-event velocity when enabled and a host integration calls `RecordPayment` for each transaction attempt.
- A new coarse payment country after warmup when enabled; the country must come from a trusted payment source.
- Expired, revoked, and exposed key-status signals, each independently enabled and supplied by trusted authentication or credential-management code.

### Does not detect

- Whether a request is malicious, who controls a key, prompt intent, semantic abuse, or account takeover. Phrase matches are weak indicators and can match benign quoted or educational text.
- Traffic hidden from the plugin pipeline or failures that occur before FraudGuard's pre-hook.
- Patterns outside the in-memory rolling window or after process restart.
- Reliable caller identity from IP headers. FraudGuard does not read IP headers and does not treat them as identity.
- Cross-key patterns by default. Optional key-rotation correlation requires an authenticated user identity and is disabled unless explicitly configured.
- Payment transactions or payment location automatically. The host must send trusted events; Bifrost's LLM request hook does not provide them.
- Whether a key is expired, revoked, or exposed automatically. The host must supply those status results; FraudGuard does not query a key registry or scan for leaked credentials.

Findings are indicators for human review, not blocking decisions or an authorization verdict. False positives are expected for legitimate model migrations, first-time workloads, retries, batch jobs, or changing token usage.

## Privacy and operational safety

- Observe-only: hooks always pass the request and response through; no automatic block path exists.
- There is no penalty box or automatic cooldown. A score above threshold is an advisory for human review, not a tenant or IP ban.
- The plugin reads only the authenticated internal virtual-key ID from `BifrostContextKeyGovernanceVirtualKeyID`. It never falls back to `x-bf-vk` or another raw credential.
- Bifrost or the host must verify user authentication and resolve identity before the hook. FraudGuard does not parse Authorization headers or accept tenant/subscription claims from an unverified JWT.
- Optional cross-key correlation uses only the resolved `Grant.Identity().User().ID`, when Bifrost supplies one. It is separately HMACed with a namespace distinct from virtual-key IDs; it never reads IP headers or request-provided fingerprints. The signal is disabled by default.
- The engine stores only a per-process HMAC digest of that internal ID as its map key. It stores bounded provider/model labels and token counts/timestamps for behavioral comparison; it does not store prompt, response, payload, key, request ID, or error text.
- Prompt checks are disabled by default. When enabled, the pre-hook scans only user text in chat, text-completion, and Responses inputs, up to `MaxPromptScanBytes`; it ignores system/developer messages, media, tool data, and raw request bodies. It passes only a boolean match and byte count to the scorer, which does not retain either in profile history.
- The optional context guard is disabled by default. When enabled, it inspects only user-role text from the current request's conversation array, with message and byte caps. It returns three fixed booleans for staged override, disclosure, and schema-change heuristics; assistant/system/developer messages and all text from the guard result are excluded.
- Findings are delivered through a bounded asynchronous callback queue so a slow audit sink does not hold up the request hook. Queue overflow drops callback delivery and increments a metric; `Cleanup` drains queued findings. Sink callbacks should still return promptly.
- Custom prompt rules are disabled by default, capped at 16 expressions of at most 256 bytes each, and run only on the bounded user-text scan. Updates atomically replace the in-memory compiled set; neither pattern strings nor prompt text are emitted or retained by findings.
- Payment subjects and coarse country codes are HMACed before storage. Payment state is bounded by subject count, event count, rolling window, and profile TTL. No payment amounts, card data, exact addresses, or raw country values are retained.
- Credential-status inputs are booleans; the engine does not receive or store credential values, expiry dates, or revocation records.
- Correlation state stores at most the configured key limit plus one HMAC key digest per cohort, and is bounded by `MaxProfiles` and `ProfileTTL`.
- Findings contain only timestamp, score, threshold, alert/warmup flags, and named signals with fixed reasons. They omit model/provider labels and all key identifiers.
- The optional callback receives only those findings. Metrics are aggregate counts.
- Optional AI review sends only score, threshold, warmup/alert flags, and fixed signal names/reasons/weights to the host adapter. It does not send prompts, payloads, provider/model labels, virtual-key IDs, payment subjects, or credential values.
- AI review results are separate, bounded advisories. They cannot change the deterministic score, threshold alert, or request flow. A full queue drops the review and increments a metric; adapter errors/timeouts are counted without exposing error text.
- The default profile store is process-local and is reset on restart. Production use across replicas requires a separately designed shared-state and retention strategy, which is not included here.
- Optional Redis product services keep tenant usage aggregates and audit records across restarts. They do **not** make detector behavior profiles shared across Bifrost replicas.
- The optional product Redis client uses go-redis's bounded connection pool and bounded dial/read/write/pool waits. It is not queried for request behavior profiles or request velocity, and no sub-millisecond latency guarantee is made.
- The optional product Redis client uses go-redis's bounded connection pool and bounded dial/read/write/pool waits. It is not queried for request behavior profiles or request velocity, and no sub-millisecond latency guarantee is made.
- No Redis Lua velocity limiter is included. Redis is not used for request counts, behavioral profiles, throttling, or blocking.
- Tenant identifiers are accepted only from the configured trusted host resolver; Redis keys and exports use HMAC-derived tenant references. Never resolve a tenant from an untrusted request header.
- Virtual-key attribution reads only Bifrost's already-resolved internal context ID. This module does not parse raw key headers, Authorization values, JWT claims, or provider API credentials.
- Usage metering and audit work runs on a bounded queue. Queue drops and service errors are counted in `ProductMetrics`; metering is therefore asynchronous, not a synchronous billing authorization check.
- Audit records are HMAC-signed and tamper-evident while retained. Redis administrators can still delete or rewrite the Redis data; this is not immutable/WORM storage.
- Webhooks contain only sanitized audit records and signatures. Delivery is bounded and retried; this module does not deliver email or SMS.

The HMAC key is randomly generated per engine process. It is not persisted or emitted. This limits correlation and means the hashed map keys cannot be joined across restarts.

## Configuration

Registering the plugin in `schemas.BifrostConfig.LLMPlugins` enables it. Prompt inspection, custom prompt rules, payment checks, key-status checks, cross-key correlation, and AI review are separate opt-ins and default off.

```go
plugin, err := fraudguard.New(fraudguard.Config{
    WarmupRequests:      8,
    Window:              20 * time.Minute,
    ProfileTTL:          3 * time.Hour,
    MaxProfiles:         20_000,
    MaxEventsPerProfile:         256,
    AlertThreshold:               60, // inclusive score threshold from 1 through 100
    ExemptVirtualKeyIDs:          []string{"resolved-internal-vk-id"},
    EnableCrossKeyCorrelation:    true, // opt in; default is false
    MaxDistinctKeysPerIdentity:   3,    // alert when the limit is exceeded
    CorrelationWindow:            10 * time.Minute,
    EnablePaymentVelocity:        true, // opt in; default is false
    EnablePaymentLocation:        true, // opt in; default is false
    MaxPaymentsPerWindow:         8,
    PaymentWindow:                15 * time.Minute,
    PaymentLocationWarmupEvents:  3,
    EnableExpiredKeySignal:       true, // each key-status signal is independent
    EnableRevokedKeySignal:       true,
    EnableExposedKeySignal:       true,
     EnablePromptOverrideSignals:  true, // opt in; text is inspected transiently
     EnablePromptLengthSignal:     true, // opt in; user text only
     MaxPromptBytes:               32_768,
     MaxPromptScanBytes:           64 * 1024,
     FindingCallbackQueueSize:     256,
    OnFinding: func(f fraudguard.Finding) {
        // Forward only this structured, payload-free finding to your audit pipeline.
        // Do not attach prompts, raw keys, or error strings here.
    },
})
if err != nil {
    return err
}
```

Call `Cleanup` during host shutdown to drain pending finding callbacks and stop optional AI workers. Finding callbacks run asynchronously from a bounded queue; a full queue drops events and increments a metric. The host should monitor `FindingCallbacksDropped` and provide a promptly returning sink.

### Startup environment overrides

Use `NewFromEnvironment` to apply the documented startup variables before plugin creation. This constructor also handles the separate optional Redis product-service settings described below. `New(config)` remains available for callers that supply configuration directly.

```go
plugin, err := fraudguard.NewFromEnvironment(fraudguard.Config{
    WarmupRequests: 8,
})
```

Supported detector overrides include:

| Variable | Accepted values |
| --- | --- |
| `FRAUDGUARD_ALERT_THRESHOLD` | Integer 1–100 |
| `FRAUDGUARD_WARMUP_REQUESTS` | Integer 1–10000 |
| `FRAUDGUARD_WINDOW`, `FRAUDGUARD_PROFILE_TTL` | Go durations, within the limits enforced by `ValidateConfig` |
| `FRAUDGUARD_MAX_PROFILES`, `FRAUDGUARD_MAX_EVENTS_PER_PROFILE` | Positive bounded integers |
| `FRAUDGUARD_ENABLE_PROMPT_OVERRIDE_SIGNALS`, `FRAUDGUARD_ENABLE_PROMPT_LENGTH_SIGNAL`, `FRAUDGUARD_ENABLE_CONTEXT_GUARD` | `true` or `false`; prompt/context checks remain opt-in |
| `FRAUDGUARD_ENABLE_PAYMENT_VELOCITY`, `FRAUDGUARD_ENABLE_PAYMENT_LOCATION` | `true` or `false`; host-supplied trusted payment events are still required |
| `FRAUDGUARD_ENABLE_EXPIRED_KEY_SIGNAL`, `FRAUDGUARD_ENABLE_REVOKED_KEY_SIGNAL`, `FRAUDGUARD_ENABLE_EXPOSED_KEY_SIGNAL` | `true` or `false`; trusted host results are still required |

`ConfigFromEnvironment` applies and validates startup overrides without constructing a plugin; `ValidateConfig` can validate a supplied config directly. Invalid explicit environment values return an error without echoing the raw value. The detector has no request-rate-limit setting or blocking mode; `MaxPaymentsPerWindow` is only an optional payment-behavior signal threshold. Since no request is blocked, there is no OpenAI-compatible blocked-request error response.

### Startup security checks and release metadata

`NewEngine` and `ConfigFromEnvironment` call `ValidateStartupSecurity` after configuration defaults/overrides are applied. `RunSecurityChecks` returns fixed status codes for config bounds, observe-only handling, trusted Bifrost identity, and bounded prompt inspection. It deliberately reports secret values as `not_inspected`: the helper does not enumerate environment variables, read secret contents, verify JWTs, or scan dependencies, and it is not a vulnerability scanner.

The authenticated health and admin JSON responses include `BuildInfo`. The values are set at link time and sanitized; no environment variables or metric labels are used:

```sh
cd plugins/fraudguard
VERSION=v1.2.3 \
COMMIT="$(git rev-parse --short=12 HEAD)" \
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  make build
```

Without a release tag or supplied build time, metadata defaults to `dev`/`unknown`. Hosts building Bifrost with the plugin as a dependency can set the same `-X github.com/maximhq/bifrost/plugins/fraudguard.BuildVersion=...`, `BuildCommit=...`, and `BuildTime=...` linker variables. No release tag is created by the module.

For direct `New(config)` calls, the existing zero/negative-value defaulting behavior remains: 5 warmup requests, a 15-minute rolling window, 2-hour idle profile TTL, at most 10,000 profiles, 256 events per profile, alert threshold 60, 32 KiB prompt-size limit, 64 KiB prompt scan cap, 32 user-message context cap, 64 KiB context scan cap, and a 256-item finding callback queue. Prompt and context checks and payment signals default off. Payment windows/TTLs inherit the main window/profile TTL, payment subject/event bounds inherit the main profile/event bounds, the velocity signal threshold defaults to 10, and location warmup defaults to three trusted events. A direct `New` config threshold above 100 is clamped to 100; `NewFromEnvironment` rejects explicit out-of-range values. Warmup suppresses baseline-based behavioral anomaly signals until enough prior events exist.

### Multi-turn context guard

Set `EnableContextGuard: true` to inspect a bounded sequence of user-role text segments in the current Bifrost request. `MaxContextMessages` defaults to 32 (maximum 256); `MaxContextScanBytes` defaults to 64 KiB (maximum 1 MiB). The local rules look for a framing phrase in an earlier user turn followed by an override phrase, disclosure-action/secret-target pairs, and JSON/tool schema markers paired with an override directive. The output is only three booleans and scan counters; no conversation text is stored, emitted, or passed to AI analysis. The guard does not carry conversation state across requests, parse or enforce a JSON schema, understand intent, or block requests. These simple phrase rules can produce false positives and are a demonstration signal, not a jailbreak guarantee.

### Custom operator prompt rules

Create a `CustomPromptRules` manager, call `ReplaceJSON` with an operator-owned JSON array of RE2 expressions, and pass it through `Config.CustomPromptRules` with `EnableCustomPromptRules: true`. The manager accepts at most 16 patterns, each at most 256 bytes, in a document at most 16 KiB. It swaps valid sets atomically; invalid updates leave the previous set active. A match adds the fixed `custom_prompt_pattern` signal only. Rules are transiently applied to bounded user text and are not stored in Redis. The host must authenticate and authorize any code that calls `ReplaceJSON`; the module exposes no remote rule-update endpoint.

### Runtime configuration reload

`NewFromEnvironment` can poll a local JSON snapshot and apply supported risk settings without restarting the gateway:

```text
FRAUDGUARD_CONFIG_PATH=/etc/fraudguard/runtime.json
FRAUDGUARD_CONFIG_POLL_INTERVAL=5s
```

The interval defaults to 5 seconds and must be between 100 milliseconds and 1 hour. Start with [`config.example.json`](config.example.json); each file must be a complete version-1 snapshot and stay below 1 MiB. Write updates atomically (write a temporary file, then rename it). The initial file must load successfully or plugin startup fails. After startup, a malformed or unsafe update is rejected while the last known good settings stay active. Hosts can inspect `plugin.ConfigReloadStatus()` for the applied generation, failure state, and a stable error code.

Reloadable values include alert threshold, warmup/window/retention bounds, opt-in signal switches, payment-velocity alert settings, prompt/context scan bounds, and internal-key exemptions. AI adapters, callbacks, worker counts, and queue sizes remain startup-only. Exemption values must be resolved internal key IDs, never bearer/API keys; restrict access to the config file. Payment velocity and context findings are advisory only. This format intentionally has no blocklist, request throttle, or auto-block rule.

### Metrics and internal diagnostics

FraudGuard provides process-wide counters and histograms without tenant, key, model, provider, prompt, request, or payment labels. The Prometheus-compatible text is available through `plugin.PrometheusText()`. To use the host's OpenTelemetry provider, call `plugin.EnableOpenTelemetry(meter)` before serving requests; the host remains responsible for exporting those instruments.

### OpenTelemetry tracing

FraudGuard uses the host's global OpenTelemetry tracer provider. Configure that provider before serving traffic; with the default no-op provider, spans are created but not exported. Spans cover pre/post hooks, opt-in prompt/context inspection, risk scoring, and optional Redis product-service operations. Attributes are limited to booleans, scan byte counts, score, signal count, and fixed failure codes. Spans never attach prompt text, raw virtual keys, tenant IDs, model/provider labels, request bodies, or raw error messages. Redis spans refer to product metering/audit work only; behavioral profiles and velocity scoring are in memory, not Redis-backed.

### Fault-injection tests

`make chaos` runs local tests for a Redis disconnect, a slow optional product ledger, circuit-breaker behavior, and pass-through of a simulated upstream timeout. These use miniredis and in-process fakes; they do not simulate a Redis Cluster, real network packet loss, an actual upstream timeout, or production traffic.

For an HTTP diagnostics handler, pass a host-owned authorization check:

```go
adminHandler, err := fraudguard.NewAdminHandler(plugin, func(r *http.Request) bool {
    return trustedAdminMiddlewareAlreadyAuthorized(r)
})
if err != nil {
    return err
}
adminMux.Handle("/admin/fraudguard/", adminHandler)
```

The handler serves authenticated `GET` requests for `/admin/fraudguard/metrics`, `/admin/fraudguard/status`, `/admin/fraudguard/dashboard`, and `/admin/fraudguard/healthz`. Bind it to a private admin listener and enforce authorization in the host. The dashboard combines a process-wide health state with aggregate engine/product metrics, config-reload health, and Redis circuit state. It does not include active-tenant identities/counts or blocked-injection totals. There is deliberately no tenant/key lookup, velocity-state listing, penalty box, or block log. FraudGuard never blocks, so no blocked-request metric is exported.

Metrics are local to one process. They are not a replacement for the separate Redis usage ledger or an external multi-replica metrics backend. The OpenTelemetry instrumentation records evaluation/completion/finding counters, evaluation duration and observed risk-score histograms, plus aggregate active-profile/cohort/payment-subject gauges.

The read-only `cmd/fraud-admin` utility can query a host-mounted, authenticated diagnostics handler:

```sh
FRAUDGUARD_ADMIN_URL=http://127.0.0.1:8081 \
FRAUDGUARD_ADMIN_TOKEN="$FRAUDGUARD_ADMIN_TOKEN" \
  go run ./cmd/fraud-admin metrics
```

It supports `health`, `status`, and `metrics` GET requests only. The host must mount `NewAdminHandler` behind its own authorization and private listener. The CLI does not expose tenant lookups, raw Redis keys, unblocking, policy mutation, or threat-feed clearing. The pinned Compose demo does not mount this admin listener, so it will not claim to fetch live FraudGuard metrics.

### Health and readiness probes

`NewProbeHandler` exposes authenticated `GET /livez`, `/healthz`, and `/readyz` routes for a host to mount on a private listener. `CheckHealth` reports process-wide heap allocation, plugin initialization, and a bounded Redis `PING` when a product service implements `RedisHealthChecker`. Redis is optional for inference readiness by default: a Redis outage appears as degraded product health, but does not interrupt observe-only inference. A host can set `MaxHeapAllocBytes` and `RequireRedisForReadiness` according to its deployment policy. Heap usage is process-wide, not a plugin-only memory estimate. The probe handler is not wired into Bifrost automatically; use the host's private listener and its supported probe authorization headers.

### Redis outage behavior

Redis is optional for metering, plans, audit, and webhooks; it is not used to keep detector profiles or to decide whether an inference request may continue. Redis commands have bounded timeouts and share a circuit breaker (default: three consecutive failures opens it for five seconds; configure `RedisStoreOptions.BreakerConfig` and `OperationTimeout` for direct store construction). Once open, Redis calls fail fast until one recovery probe is allowed. The status exposes only the circuit state and a stable failure code.

The environment-backed constructor skips the startup `PING` and warms plan data best-effort. This keeps an unavailable optional Redis service from preventing the observe-only plugin from starting. Inference remains fail-open; usage/audit events can be incomplete during an outage, and their failures/drop counts remain observable. This deliberately does not offer fail-closed request handling because it would let an optional product service interrupt or block model requests.

### Optional Redis product services

The default `New` constructor uses only process-local detection state. `NewFromEnvironment` enables shared tenant metering/audit services only when the required Redis settings are present. With any partial Redis configuration, initialization fails explicitly:

```text
FRAUDGUARD_REDIS_URL=redis://...
FRAUDGUARD_TENANT_HMAC_KEY=<at least 32 bytes>
FRAUDGUARD_TENANT_ID=<trusted deployment tenant fallback>
```

Set these through the deployment's secret manager, not in source control. In a multi-tenant host, set the tenant with `SetTrustedTenantID` from authenticated server-side context; the environment tenant is only a single-company fallback. The plugin never trusts a caller-supplied tenant header.

Optional `FRAUDGUARD_TIER_POLICIES_JSON` is a JSON array of operator-owned policies. The limits below are examples only, not recommended prices or quotas:

```json
[
  {
    "tier": "free",
    "monthly_request_limit": 10000,
    "monthly_token_limit": 1000000,
    "max_payments_per_window": 10,
    "alert_threshold": 60,
    "warning_percent": 80,
    "features": {
      "prompt_signals": false,
      "payment_signals": false,
      "key_status_signals": false,
      "cross_key_signals": false,
      "ai_review": false,
      "usage_export": true,
      "webhook_alerts": false
    }
  }
]
```

When tier policies are enabled, the configured tenant must already have a tier assignment in Redis before plugin startup. `RedisProductStore.SetTenantTier` is the host-side provisioning hook. Cached policy applies alert thresholds, opted-in prompt/payment/key-status/cross-key signals, optional AI review, usage-export entitlement, webhook entitlement, and payment-velocity limits without blocking requests. The environment constructor warms its configured tenant at startup. A multi-tenant host should warm each trusted tenant with `ProductRuntime.ResolvePolicy` before serving traffic; a cold cache leaves plan-only signals disabled and increments `PolicyCacheMisses`. Policy refresh is asynchronous for inference requests; use the host's trusted plan service for authoritative subscription changes. The in-memory tier cache is TTL-based and LRU-evicted at a configurable maximum (`ProductServices.MaxCachedPolicies`, default 10,000, maximum 100,000).

The custom prompt-rule set is compiled once and atomically replaced; it does not require a repeated lookup cache. No IP blocklist exists, so there is no IP-policy cache.

Optional webhook settings are `FRAUDGUARD_WEBHOOK_URL`, `FRAUDGUARD_WEBHOOK_SIGNING_KEY` (at least 32 bytes), and `FRAUDGUARD_WEBHOOK_ALLOWED_HOSTS` (comma-separated exact hostnames). All three are required together. The endpoint must be HTTPS and allowlisted; redirects and private-network destinations are rejected by default. The environment webhook is scoped to the configured deployment tenant. Multi-tenant delivery requires a host-supplied `WebhookTargetResolver`.

The usage ledger deduplicates repeated writes with the same event ID. The Bifrost hook creates a fresh event ID per invocation, so gateway retry attempts are not deduplicated by this module. The usage export helper emits versioned JSON with aggregate request/token/fraud-check counts and an optional plan evaluation. It intentionally contains no price or `blocked_requests` field because FraudGuard never blocks. Export does not create invoices, charge cards, or call Stripe.

`NewBillingExportHandler` serves `GET /admin/fraudguard/usage-export?month=YYYY-MM` when the host mounts it. The host must provide an authorization callback and a tenant resolver backed by authenticated server-side context; caller-supplied tenant query parameters are rejected and tenant headers are not read by FraudGuard. The handler returns no-store JSON and applies plan export entitlements when configured. The host must mount it on a private listener and connect any billing service separately.

Payment velocity counts each trusted `RecordPayment` call. Deduplicating retried payment events remains the responsibility of the host's idempotency layer and is not implemented in this module.

### Advisory threat indicators and budget monitoring

`ThreatIntelFeed` is an opt-in, in-memory advisory feed. Trusted host code may call `ReplaceInternalKeyIDs` with already-resolved internal virtual-key IDs. FraudGuard HMACs each value immediately, applies a bounded TTL and capacity, and emits only the fixed `trusted_threat_indicator` signal on a match. It does not inspect IP addresses, user-agent strings, or request headers, and it never drops or blocks a request. The host is responsible for verifying and refreshing any source feed; no external threat-intelligence service is contacted.

`BudgetMonitor` is also opt-in. A host may attach it with `Plugin.SetBudgetMonitor` and call `Plugin.ObserveBudget` using trusted token counts and a trusted estimated cost in integer micro-USD. FraudGuard does not invent provider prices or fetch billing data. The result contains warning/over-limit indicators for reporting only; it has no authorization decision and cannot stop an inference. Its HMAC-keyed period aggregates are bounded and process-local, reset on restart, and are not a billing system or financial source of truth.

`SignWebhookPayload` and `VerifyWebhookPayload` use HMAC-SHA256 over the exact serialized body, require a 32-byte key, and verify with a constant-time comparison. Receivers should verify before decoding, then separately enforce event freshness/uniqueness: the signature does not prevent replay.

### Optional host-model review

AI review is off by default. When enabled, `AIAnalyzer` and `OnAIReview` are required. The adapter receives only `AIAnalysisInput` fields: deterministic score/threshold, alert/warmup flags, and fixed signals. It does not receive prompt or payload text, identifiers, or provider/model labels. This is an AI interpretation of existing signals, not an independent detector; it cannot discover patterns the deterministic rules did not identify. Reviews are queued asynchronously only when the score reaches `AIAnalysisMinScore` (default: `AlertThreshold`); the bounded queue and worker count default to 64 items and one worker. The model timeout defaults to five seconds.

```go
plugin, err := fraudguard.New(fraudguard.Config{
    EnableAIAnalysis: true,
    AIAnalyzer:       hostModelAdapter, // host-defined implementation using an isolated Bifrost client
    OnAIReview: func(event fraudguard.AIReviewEvent) {
        // Store or display this advisory separately from the deterministic finding.
    },
})
```

The host adapter should honor its context timeout and use a Bifrost client that does not register FraudGuard, or the model request could recursively trigger analysis. The plugin has no direct model client and never reads or stores provider credentials. The standalone demo and tests do not contact a live model; tests use a local test adapter. Call `Cleanup` to drain and stop AI review workers. AI confidence is descriptive, not a calibrated fraud probability.

The allowlist accepts internal virtual-key **IDs**, not virtual-key secrets. IDs are HMACed at initialization and never retained in clear text by the engine. Do not populate it with bearer tokens.

Cross-key correlation is off by default. When enabled, its default limit is three distinct keys and its default window matches `Window`. The Bifrost adapter uses `Grant.Identity().User().ID` only when present. Bifrost may not expose a user identity for a virtual key even when an operator considers that key assigned to a user; in that case no cross-key correlation occurs. Requests are still observed and never blocked.

Direct `Engine` users may pass a `Request.CohortID`, but only from a trusted, already-authenticated identity source. Do not derive it from IP headers, request headers, prompts, or other caller-controlled fields.

Payment and credential-status signals are disabled unless explicitly enabled. `RecordPayment` must be called once per transaction attempt with a stable internal or pseudonymous subject ID and, if location scoring is enabled, a coarse country code from a trusted payment source:

```go
paymentFinding := fraudGuard.RecordPayment(fraudguard.PaymentEvent{
    SubjectID:   pseudonymousPaymentSubject,
    CountryCode: trustedProcessorCountry,
    Timestamp:   time.Now(),
})
```

The Bifrost request hook does not discover payment events. Do not pass card numbers, payment tokens, addresses, IP-derived caller headers, or raw customer credentials. For key status, trusted authentication or credential-management code can attach only the status booleans to the current Bifrost context before FraudGuard's `PreLLMHook`:

```go
fraudguard.SetTrustedCredentialStatus(ctx, fraudguard.CredentialStatus{
    Expired: statusFromTrustedRegistry.Expired,
    Revoked: statusFromTrustedRegistry.Revoked,
    Exposed: statusFromTrustedRegistry.MarkedExposed,
})
```

`Exposed` means another trusted system has already marked the key as exposed; FraudGuard does not search public sources or inspect secrets. These findings never authorize, reject, or alter a request or payment.

## Installation & Setup

From the repository root, `make help` lists convenience commands such as `make fraudguard-test`, `make fraudguard-chaos`, `make fraudguard-demo`, `make fraudguard-compose-demo`, and `make fraudguard-load-test`. From this module directory, `make setup` checks Go and runs local verification without creating `.env` files or pulling containers. The explicit `bash scripts/setup_dev.sh --compose-demo` option runs the pinned disposable Bifrost/Redis/mock-provider smoke test and requires Docker plus network access.

### Run the module locally

Use Go 1.27.3. Clone the public repository, then run:

```sh
git clone https://github.com/sham-cyber/Plugin.git
cd Plugin
cd plugins/fraudguard
make help
make verify
```

`make verify` runs tests, the race detector, vet, and the synthetic demo. `make test`, `make race`, `make vet`, and `make demo` run those steps individually. `make redis-check` optionally verifies `FRAUDGUARD_REDIS_URL`; it does not print the URL or credentials. `make integration BIFROST_ROOT=/path/to/bifrost` verifies against the exact pinned source checkout. The demo uses synthetic data and makes no service calls. These commands test and demonstrate the companion module; they do not install it into a running Bifrost server.

The FraudGuard web page's **Run Go detector** action calls `GET /api/fraudguard/demo`. The local API server runs `go run ./cmd/demo -json` and validates the response before returning it. The endpoint feeds fixed synthetic cases through the actual Go engine; it does not receive company traffic, credentials, or provider requests. The API server therefore needs the Go toolchain available. The separate company-dashboard page remains a synthetic preview and is not connected to this endpoint.

### Manually integrate with a Bifrost checkout

This project is not a Bifrost source checkout, so the module lives in `plugins/fraudguard` and pins Bifrost's core API. From this project's root, the local overlay can be applied to a separate Bifrost checkout at the exact pinned commit:

```sh
export BIFROST_ROOT=/path/to/bifrost
bash plugins/fraudguard/integration/apply-bifrost-integration.sh --apply
cd "$BIFROST_ROOT/transports"
GOSUMDB=sum.golang.org go test -p 1 ./bifrost-http/server
```

The script verifies the commit before copying the module, registering the built-in plugin name and loader, adding local sibling-module replacements, and formatting the touched Go files. `--apply` is required. It changes only that local checkout, refuses a different commit or a conflicting plugin directory, and never commits or pushes. Review `git diff` before using the result. The integration test checks the actual Bifrost loader and verifies that the hook returns the same request without short-circuiting.

Applying this overlay is a local integration demonstration, not an upstream release. It does not mean the plugin is installed, accepted, or merged in the public Bifrost repository. Direct `core.Init` registration remains another supported integration path. If AI review is enabled, the host must supply an `AIAnalyzer` backed by its own isolated Bifrost model client.

### Containerized pinned-source smoke demo

From the project root, with Docker Engine and the Compose plugin installed:

```sh
bash plugins/fraudguard/scripts/run-compose-demo.sh
```

The script fetches Bifrost commit `54ad3abec4fbbdaedf59414415094760fb9aa7e9` into an isolated temporary directory under the ignored `generated-artifacts/` folder, applies the local overlay only to that disposable checkout, builds a Bifrost image, and starts it with Redis and the local synthetic OpenAI-compatible mock provider. It does not edit a separate Bifrost checkout or push anything upstream. The synthetic virtual key is `sk-bf-fraudguard-local-demo`; the provider returns a fixed local response and never echoes or logs prompts. Use `Ctrl+C` to stop the demo; its temporary checkout and Compose containers are removed.

Send a synthetic request in a second terminal:

```sh
curl http://localhost:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'x-bf-vk: sk-bf-fraudguard-local-demo' \
  -d '{"model":"vllm-local/model-a","messages":[{"role":"user","content":"synthetic demo request"}]}'
```

Send another request with a different `vllm-local/model-*` name to exercise a model-novelty case. The HTTP response is the mock provider's fixed response; this container harness does not add a tenant-facing findings UI or expose Redis contents. Use `go run ./cmd/demo` for directly printed synthetic scores and signal names. The Compose configuration is a local integration/smoke harness, not production deployment evidence; building it requires internet access to fetch the pinned source and container images.

### Judge walkthrough

From the project root:

```sh
bash plugins/fraudguard/scripts/demo_walkthrough.sh --compose
```

This runs Go tests, builds the temporary pinned Bifrost/Redis/mock-provider stack, sends a benign request and a synthetic multi-turn anomaly, and requires both to receive HTTP 200 to demonstrate observe-only pass-through. It then prints the local synthetic detector output. Set `FRAUDGUARD_ADMIN_URL` only if a separately mounted, authenticated admin endpoint is available; otherwise the script says that no live metrics endpoint was queried. Use `--local` to run tests and detector output without Docker. The script does not claim that a request was blocked or that local synthetic scores came from the live gateway.

Minimal registration example:

```go
fraudGuard, err := fraudguard.New(fraudguard.Config{
    WarmupRequests: 5,
    AlertThreshold: 60,
    OnFinding: func(f fraudguard.Finding) {
        // Send a structured finding to your operator-owned sink.
    },
})
if err != nil {
    return err
}

client, err := bifrost.Init(ctx, schemas.BifrostConfig{
    Account:   account,
    LLMPlugins: []schemas.LLMPlugin{fraudGuard},
})
if err != nil {
    return err
}
_ = client
```

For repeated denied/malformed outcomes to reach FraudGuard's post-hook, put it before a plugin that may short-circuit in `PreLLMHook`, while preserving the deployment's intended plugin order. The resolved internal virtual-key identity must already be set by the authentication/governance path. Requests without that identity are passed through and are not profiled.

## Demo

From this directory:

```sh
go run ./cmd/demo
```

The demo warms a key with five requests 30 seconds apart, then simulates another request after one second. It also supplies synthetic payment events and trusted key-status booleans to the optional signal APIs. It prints observe-only findings and aggregate metrics; it does not enable AI review or call an external service.

Expected synthetic signals include `rapid_sequence`, `payment_new_location`, `payment_velocity`, and the separate `key_expired`, `key_revoked`, and `key_exposed` signals.

## Tests

```sh
go test ./...
go test -race ./...
go vet ./...
```

Tests cover benign baselines, below-rate-limit anomalies, warmup, deterministic scores/reasons, token spikes, repeated failures, prompt/context-check opt-in and scan bounds, prompt privacy, concurrent requests/payment/AI reviews, expiry and profile bounds, exemptions, threshold behavior, tenant plan overrides, Redis usage idempotency, tier evaluation, usage exports, audit signatures, webhook target validation, hot reload/rejection/recovery, Prometheus output, OpenTelemetry instrument registration, admin authorization, Redis circuit transitions, and concurrent config updates. `main_test.go` sends a request through Bifrost's real core plugin pipeline to a local mock HTTP provider and verifies that an alerting prompt anomaly still reaches the provider. It does not start Bifrost's inbound HTTP gateway server; the separate overlay test checks the pinned server loader and hook pass-through. Redis behavior is tested with miniredis and local unavailable-port fixtures; no live Redis, webhook endpoint, billing provider, or AI model is contacted.

Context tests cover multi-turn staged patterns, JSON/tool-schema marker combinations, ignored non-user roles, byte/message bounds, raw-text privacy, request pass-through, and fixed risk scoring. CLI tests cover authenticated aggregate GET requests, rejection of mutation commands, and safe handling of unauthorized responses. Benchmarks measure the in-process engine and guard only:

Custom-rule tests cover RE2 compilation, bounded atomic replacement, invalid-update retention, user-text-only inspection, static findings without prompt/rule/key values, and advisory pass-through. Dashboard tests cover authorization and the absence of per-tenant or blocked-request fields. The optional k6 script targets loopback by default, caps duration/VUs, and measures whole-gateway behavior rather than isolated plugin latency; it has not been run unless k6 and the local mock gateway are available.

```sh
make bench
go test -run '^$' -bench 'BenchmarkEngine|BenchmarkInspectConversationContext' -benchmem -cpu 1,4
```

Benchmark results depend on hardware and do not establish a sub-5-ms gateway SLO. They exclude the Bifrost HTTP server, Redis, provider/network latency, and deployment scheduling.

### Verify against the exact upstream commit

To verify the plugin against the pinned Bifrost core source instead of the published core module, clone Bifrost and check out the commit recorded above, then run this from the plugin directory:

```sh
BIFROST_ROOT=/path/to/bifrost bash scripts/verify-pinned-bifrost.sh
```

That script refuses a different Bifrost commit, creates a temporary Go workspace outside both repositories, and runs tests, the race detector, `go vet`, and the demo against the local `core` source without modifying the checkout. The built-in server-loader test is separate: apply the integration overlay above, then run `go test -p 1 ./bifrost-http/server` from `transports`.

## Scoring signals

Signal weights add and the total is capped at 100. Signals are sorted by name for deterministic output.

| Signal | Weight | Rule of thumb |
| --- | ---: | --- |
| `new_model` | 25 | Model label was not observed for this key in the active history. |
| `new_provider` | 25 | Provider label was not observed for this key in the active history. |
| `rapid_sequence` | 30 | Current inter-request gap is under one quarter of the median prior gap and under one tenth of the configured window. |
| `key_rotation` | 30 | An opted-in trusted cohort exceeds its distinct-key limit inside the correlation window; emitted once when the limit is crossed. |
| `input_volume_spike` | 30 | Input tokens exceed 3x the prior mean or prior mean + 512, with at least two completed samples. |
| `output_volume_spike` | 25 | Output tokens exceed 3x the prior mean or prior mean + 256, with at least two completed samples. |
| `repeated_errors` | 35 | Among the last five completed outcomes, at least three fail and failures are at least 60%; the earlier retained baseline error rate must be no more than 50%. |
| `payment_velocity` | 60 | An opted-in trusted payment subject exceeds `MaxPaymentsPerWindow` inside `PaymentWindow`. |
| `payment_new_location` | 35 | After the configured warmup, an opted-in trusted country code has not appeared in that subject's retained payment history. |
| `key_expired` | 70 | Enabled and supplied by trusted credential-status code. |
| `key_revoked` | 70 | Enabled and supplied by trusted credential-status code. |
| `key_exposed` | 70 | Enabled and supplied by trusted credential-status code. |
| `prompt_override_phrase` | 40 | Optional local phrase heuristic over bounded user text; default off. |
| `prompt_size_spike` | 25 | Optional user-text byte count exceeds `MaxPromptBytes`; default off. |
| `context_progressive_jailbreak` | 35 | An earlier user turn contains a framing phrase and a later user turn contains an override phrase; opt-in. |
| `context_data_exfiltration` | 45 | A bounded user turn pairs a disclosure action with a sensitive-data target; opt-in. |
| `context_schema_manipulation` | 35 | A bounded user turn pairs JSON/tool schema markers with an override directive; opt-in. |

The numerical rules are intentionally simple and inspectable; they are not calibrated production fraud models. Prompt phrase checks are especially prone to false positives. Operators should tune warmup/window/threshold against representative traffic and review alert rates before relying on the findings.

## Limitations

- In-memory state is per process and non-durable. Multiple Bifrost replicas do not share baselines.
- Model/provider distribution is represented by novelty rather than a learned statistical distribution.
- Token counts are available only for responses that report normalized usage. Missing usage cannot produce token-spike signals.
- Payment and key-status signals require explicit host inputs. No payment processor, location service, or credential-status registry is connected here; the Bifrost request path alone cannot produce them.
- There is no IP/tenant blocklist API or Redis-backed policy mutation endpoint. Runtime exemptions accept trusted resolved internal virtual-key IDs; IP and user-agent headers are not treated as identity.
- Context rules inspect only a bounded set of user-role text segments in one request. They do not inspect prior requests, do not make semantic judgments, and can misclassify quoted or educational examples.
- Failed requests observed before a response contribute to the error signal only if Bifrost runs this plugin's post-hook for that request.
- Cross-key correlation depends on Bifrost supplying a resolved authenticated user identity; it does not infer relationships between keys from headers or network addresses.
- Periodic profile expiration is available via `Engine.Prune`; profile count is also bounded with oldest-profile eviction.
- The Go plugin module has no live tenant-facing dashboard, provider-specific AI client, payment processor, billing API, or email/SMS delivery. AI review requires a host-supplied adapter and is disabled by default. The separate company-facing page in this project remains a synthetic visual prototype with no live tenant/subscription system or plugin feed.
- The Redis usage ledger and audit stream are optional product services, not a shared behavioral profile store. Redis outages or queue overflow can leave usage/audit data incomplete; monitor `ProductMetrics` and do not use this ledger as the sole financial source of truth.
- Trusted internal-key threat indicators and budget warnings are optional advisory helpers. They do not use IP/user-agent fingerprints, do not perform external feed lookups, and do not block requests. Budget estimates require host-supplied trusted pricing and usage data.
- `docker-compose.yml` can build a local Bifrost image from the pinned source with the local integration overlay and a synthetic provider. The image build and complete Compose runtime have not been executed as part of this verification; the Compose configuration itself has been validated.
- A plan limit creates a warning/overage audit event and optional webhook alert only. Requests and payments are never blocked, and exports deliberately report no blocked-request count.
- The adapter contract is tested with an in-process test implementation. A live Bifrost model client has not been connected or tested, and latency under real model traffic is unmeasured.
- The admin CLI depends on the host mounting `NewAdminHandler` with authorization; the Bifrost loader overlay does not expose an admin endpoint. The judge walkthrough separates live mock-gateway pass-through from in-process synthetic detector output.
- Benchmarks are source-level measurements only and are not a production latency guarantee.

## Related documents

- [Verification report](../../VERIFICATION_REPORT.md) — pinned-source inspection, checks run, results, and limits of verification.
