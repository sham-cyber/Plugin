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

### Does not detect

- Whether a request is malicious, who controls a key, prompt intent, semantic abuse, or account takeover.
- Traffic hidden from the plugin pipeline or failures that occur before FraudGuard's pre-hook.
- Patterns outside the in-memory rolling window or after process restart.
- Reliable caller identity from IP headers. FraudGuard does not read IP headers and does not treat them as identity.
- Cross-key patterns by default. Optional key-rotation correlation requires an authenticated user identity and is disabled unless explicitly configured.

Findings are indicators for human review, not blocking decisions or an authorization verdict. False positives are expected for legitimate model migrations, first-time workloads, retries, batch jobs, or changing token usage.

## Privacy and operational safety

- Observe-only: hooks always pass the request and response through; no automatic block path exists.
- The plugin reads only the authenticated internal virtual-key ID from `BifrostContextKeyGovernanceVirtualKeyID`. It never falls back to `x-bf-vk` or another raw credential.
- Optional cross-key correlation uses only the resolved `Grant.Identity().User().ID`, when Bifrost supplies one. It is separately HMACed with a namespace distinct from virtual-key IDs; it never reads IP headers or request-provided fingerprints. The signal is disabled by default.
- The engine stores only a per-process HMAC digest of that internal ID as its map key. It stores bounded provider/model labels and token counts/timestamps for behavioral comparison; it does not store prompt, response, payload, key, request ID, or error text.
- Correlation state stores at most the configured key limit plus one HMAC key digest per cohort, and is bounded by `MaxProfiles` and `ProfileTTL`.
- Findings contain only timestamp, score, threshold, alert/warmup flags, and named signals with fixed reasons. They omit model/provider labels and all key identifiers.
- The optional callback receives only those findings. Metrics are aggregate counts.
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
    OnFinding: func(f fraudguard.Finding) {
        // Forward only this structured, payload-free finding to your audit pipeline.
        // Do not attach prompts, raw keys, or error strings here.
    },
})
if err != nil {
    return err
}
```

Zero or negative numeric values use the defaults: 5 warmup requests, a 15-minute rolling window, 2-hour idle profile TTL, at most 10,000 profiles, 256 events per profile, and alert threshold 60. A threshold above 100 is clamped to 100. Warmup suppresses anomaly signals until enough prior events exist.

The allowlist accepts internal virtual-key **IDs**, not virtual-key secrets. IDs are HMACed at initialization and never retained in clear text by the engine. Do not populate it with bearer tokens.

Cross-key correlation is off by default. When enabled, its default limit is three distinct keys and its default window matches `Window`. The Bifrost adapter uses `Grant.Identity().User().ID` only when present. Bifrost may not expose a user identity for a virtual key even when an operator considers that key assigned to a user; in that case no cross-key correlation occurs. Requests are still observed and never blocked.

Direct `Engine` users may pass a `Request.CohortID`, but only from a trusted, already-authenticated identity source. Do not derive it from IP headers, request headers, prompts, or other caller-controlled fields.

## Integration

This project is not a Bifrost source checkout, so the plugin lives as a sibling-style module in `plugins/fraudguard` and pins Bifrost's core API. To try it in a checkout of the upstream repository:

1. Copy this `plugins/fraudguard` directory into the matching directory in your Bifrost checkout at the pinned commit.
2. Add the module to the app/server's Go workspace or module graph using the same approach as its other plugin modules.
3. Construct `fraudguard.New(...)` and include the result in `schemas.BifrostConfig.LLMPlugins` when calling `core.Init`.

This direct `core.Init` integration does not add FraudGuard to the upstream HTTP server's built-in plugin list. That server has its own built-in plugin loader and configuration path. Making FraudGuard a built-in would require a separate upstream integration change; no upstream files have been modified for this prototype.

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

The demo warms a key with five requests 30 seconds apart, then simulates another request after one second. It prints an observe-only rapid-sequence finding and aggregate metrics. It does not send any network traffic.

## Tests

```sh
go test ./...
go test -race ./...
```

Tests cover a benign baseline, an anomaly below a simple 10-requests/minute limit, warmup, deterministic scores/reasons, token spikes, repeated failures, concurrent requests, expiry and profile bounds, exemptions, threshold behavior, privacy of emitted findings, and the Bifrost hook's observe-only behavior.

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

The numerical rules are intentionally simple and inspectable; they are not calibrated production fraud models. Operators should tune warmup/window/threshold against representative traffic and review alert rates before relying on the findings.

## Limitations

- In-memory state is per process and non-durable. Multiple Bifrost replicas do not share baselines.
- Model/provider distribution is represented by novelty rather than a learned statistical distribution.
- Token counts are available only for responses that report normalized usage. Missing usage cannot produce token-spike signals.
- Failed requests observed before a response contribute to the error signal only if Bifrost runs this plugin's post-hook for that request.
- Cross-key correlation depends on Bifrost supplying a resolved authenticated user identity; it does not infer relationships between keys from headers or network addresses.
- Periodic profile expiration is available via `Engine.Prune`; profile count is also bounded with oldest-profile eviction.
- No dashboard, external AI service, database, network call, or auto-blocking behavior is included.

## Related documents

- [Verification report](../../VERIFICATION_REPORT.md) — pinned-source inspection, checks run, results, and limits of verification.
