# FraudGuard: Bifrost behavioral-abuse plugin prototype

FraudGuard is an experimental, observe-only Go plugin for the Bifrost AI gateway. It scores behavioral changes that request-count limits can miss, such as a sudden provider/model probe, a sharp usage-volume change, unusually fast requests relative to a key's own history, or a burst of failures.

This code is a companion module in this Replit project. It is **not** included in upstream Bifrost, and no upstream acceptance or merge is claimed.

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

- **Go:** version 1.27.3 for the plugin module.
- **Bifrost:** `github.com/maximhq/bifrost/core` v1.14.1 and the `schemas.LLMPlugin` contract.
- **Privacy/state:** Go standard-library HMAC/SHA-256 and synchronization primitives with bounded, in-memory profiles.
- **Optional AI review:** a host-supplied adapter using the Bifrost host's selected model; no provider-specific model client or API key is bundled in this module.
- **Verification:** `go test`, `go test -race`, `go vet`, and a script that can test against the pinned Bifrost source checkout.

The plugin contains no provider-specific AI client. With AI review disabled it makes no AI model calls; when enabled, the host-supplied adapter owns the Bifrost model call. It does not call a payment service, database, or alert provider. Go may access the module proxy to download dependencies during setup.

## Related research context

FraudGuard adapts related behavioral anti-fraud ideas from Indian Patent Application No. 202521074434 (publication no. IN202521074434 A1), titled “AI-money maker: intelligent automated revenue generation system with anti-fraud transaction detection by AI.” The application describes a broader system; this is an experimental Bifrost plugin applying related behavior-scoring ideas to AI-gateway requests. This reference does not claim the application was granted or that its claims cover FraudGuard; status and claim coverage have not been independently verified.

## Threat model

FraudGuard is intended to help operators investigate abuse of an authenticated Bifrost virtual key when each individual request remains below a fixed rate limit. It looks for shifts relative to that key's recent history, not proof of malicious intent.

### Detects

- A provider or model not observed for that key during its retained rolling window.
- Request timing that is substantially faster than the key's own recent typical interval.
- Large changes in completed input/output token volume.
- Repeated failures that dominate recent outcomes, including malformed or denied requests when those outcomes reach the plugin's post-hook.
- Exemptions for resolved internal virtual-key IDs, applied before profile creation.
- Excessive distinct virtual keys associated with a trusted authenticated user identity, when explicitly enabled.
- Payment-event velocity when enabled and a host integration calls `RecordPayment` for each transaction attempt.
- A new coarse payment country after warmup when enabled; the country must come from a trusted payment source.
- Expired, revoked, and exposed key-status signals, each independently enabled and supplied by trusted authentication or credential-management code.

### Does not detect

- Whether a request is malicious, who controls a key, prompt intent, semantic abuse, or account takeover.
- Traffic hidden from the plugin pipeline or failures that occur before FraudGuard's pre-hook.
- Patterns outside the in-memory rolling window or after process restart.
- Reliable caller identity from IP headers. FraudGuard does not read IP headers and does not treat them as identity.
- Cross-key patterns by default. Optional key-rotation correlation requires an authenticated user identity and is disabled unless explicitly configured.
- Payment transactions or payment location automatically. The host must send trusted events; Bifrost's LLM request hook does not provide them.
- Whether a key is expired, revoked, or exposed automatically. The host must supply those status results; FraudGuard does not query a key registry or scan for leaked credentials.

Findings are indicators for human review, not blocking decisions or an authorization verdict. False positives are expected for legitimate model migrations, first-time workloads, retries, batch jobs, or changing token usage.

## Privacy and operational safety

- Observe-only: hooks always pass the request and response through; no automatic block path exists.
- The plugin reads only the authenticated internal virtual-key ID from `BifrostContextKeyGovernanceVirtualKeyID`. It never falls back to `x-bf-vk` or another raw credential.
- Optional cross-key correlation uses only the resolved `Grant.Identity().User().ID`, when Bifrost supplies one. It is separately HMACed with a namespace distinct from virtual-key IDs; it never reads IP headers or request-provided fingerprints. The signal is disabled by default.
- The engine stores only a per-process HMAC digest of that internal ID as its map key. It stores bounded provider/model labels and token counts/timestamps for behavioral comparison; it does not store prompt, response, payload, key, request ID, or error text.
- Payment subjects and coarse country codes are HMACed before storage. Payment state is bounded by subject count, event count, rolling window, and profile TTL. No payment amounts, card data, exact addresses, or raw country values are retained.
- Credential-status inputs are booleans; the engine does not receive or store credential values, expiry dates, or revocation records.
- Correlation state stores at most the configured key limit plus one HMAC key digest per cohort, and is bounded by `MaxProfiles` and `ProfileTTL`.
- Findings contain only timestamp, score, threshold, alert/warmup flags, and named signals with fixed reasons. They omit model/provider labels and all key identifiers.
- The optional callback receives only those findings. Metrics are aggregate counts.
- Optional AI review sends only score, threshold, warmup/alert flags, and fixed signal names/reasons/weights to the host adapter. It does not send prompts, payloads, provider/model labels, virtual-key IDs, payment subjects, or credential values.
- AI review results are separate, bounded advisories. They cannot change the deterministic score, threshold alert, or request flow. A full queue drops the review and increments a metric; adapter errors/timeouts are counted without exposing error text.
- The default profile store is process-local and is reset on restart. Cross-process behavior requires a separately designed privacy and retention strategy and is outside this MVP.

The HMAC key is randomly generated per engine process. It is not persisted or emitted. This limits correlation and means the hashed map keys cannot be joined across restarts.

## Configuration

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
    OnFinding: func(f fraudguard.Finding) {
        // Forward only this structured, payload-free finding to your audit pipeline.
        // Do not attach prompts, raw keys, or error strings here.
    },
})
if err != nil {
    return err
}
```

Zero or negative numeric values use the defaults: 5 warmup requests, a 15-minute rolling window, 2-hour idle profile TTL, at most 10,000 profiles, 256 events per profile, and alert threshold 60. Payment signals also default off; their window/TTL inherit the main window/profile TTL, their subject/event bounds inherit the main profile/event bounds, their velocity limit defaults to 10, and location warmup defaults to three trusted events. A threshold above 100 is clamped to 100. Warmup suppresses anomaly signals until enough prior events exist.

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

### Run the prototype locally

Use Go 1.27.3. Clone the public repository, then run:

```sh
git clone https://github.com/sham-cyber/Plugin.git
cd Plugin
cd plugins/fraudguard
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/demo
```

The demo uses synthetic data and makes no service calls. These commands test and demonstrate the companion module; they do not install it into a running Bifrost server.

### Manually integrate with a Bifrost checkout

This project is not a Bifrost source checkout, so the plugin lives as a sibling-style module in `plugins/fraudguard` and pins Bifrost's core API. To integrate it manually in a checkout of the upstream repository:

1. Copy this `plugins/fraudguard` directory into the matching directory in your Bifrost checkout at the pinned commit.
2. Add the module to the app/server's Go workspace or module graph using the same approach as its other plugin modules.
3. Construct `fraudguard.New(...)` and include the result in `schemas.BifrostConfig.LLMPlugins` when calling `core.Init`.

This direct `core.Init` integration does not add FraudGuard to the upstream HTTP server's built-in plugin list. That server has its own built-in plugin loader and configuration path. Making FraudGuard a built-in would require a separate upstream integration change; no upstream files have been modified for this prototype. If AI review is enabled, the host must also supply an `AIAnalyzer` backed by its own isolated Bifrost model client.

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

The demo warms a key with five requests 30 seconds apart, then simulates another request after one second. It also supplies synthetic payment events and trusted key-status booleans to the optional signal APIs. It prints observe-only findings and aggregate metrics; it does not enable AI review or send any network traffic.

Expected synthetic signals include `rapid_sequence`, `payment_new_location`, `payment_velocity`, and the separate `key_expired`, `key_revoked`, and `key_exposed` signals.

## Tests

```sh
go test ./...
go test -race ./...
go vet ./...
```

Tests cover benign baselines, below-rate-limit anomalies, warmup, deterministic scores/reasons, token spikes, repeated failures, concurrent requests/payment/AI reviews, expiry and profile bounds, exemptions, threshold behavior, privacy of emitted findings and AI inputs, independent signal opt-ins, bounded AI queue behavior, adapter failure handling, and observe-only hooks. AI tests use an in-process test adapter, not a live model.

### Verify against the exact upstream commit

To verify the plugin against the pinned Bifrost source instead of the published core module, clone Bifrost and check out the commit recorded above, then run this from the plugin directory:

```sh
BIFROST_ROOT=/path/to/bifrost bash scripts/verify-pinned-bifrost.sh
```

The script refuses a different Bifrost commit, creates a temporary Go workspace outside both repositories, then runs tests, the race detector, `go vet`, and the demo against the local `core` source. It does not modify or commit anything in the Bifrost checkout.

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

The numerical rules are intentionally simple and inspectable; they are not calibrated production fraud models. Operators should tune warmup/window/threshold against representative traffic and review alert rates before relying on the findings.

## Limitations

- In-memory state is per process and non-durable. Multiple Bifrost replicas do not share baselines.
- Model/provider distribution is represented by novelty rather than a learned statistical distribution.
- Token counts are available only for responses that report normalized usage. Missing usage cannot produce token-spike signals.
- Payment and key-status signals require explicit host inputs. No payment processor, location service, or credential-status registry is connected in this prototype; the Bifrost request path alone cannot produce them.
- Failed requests observed before a response contribute to the error signal only if Bifrost runs this plugin's post-hook for that request.
- Cross-key correlation depends on Bifrost supplying a resolved authenticated user identity; it does not infer relationships between keys from headers or network addresses.
- Periodic profile expiration is available via `Engine.Prune`; profile count is also bounded with oldest-profile eviction.
- The Go plugin module has no dashboard, provider-specific AI client, database, alert provider, or auto-blocking behavior. AI review requires a host-supplied adapter and is disabled by default. A separate company-facing page in this project is a synthetic visual prototype only; it has no live tenant/subscription system, plugin feed, or email/SMS delivery.
- The adapter contract is tested with an in-process test implementation. A live Bifrost model client has not been connected or tested, and latency under real model traffic is unmeasured.

## Related documents

- [Verification report](../../VERIFICATION_REPORT.md) — pinned-source inspection, checks run, results, and limits of verification.
