# FraudGuard presentation guide

This file is separate from the technical README. It gives a short talk track and ready answers for a hackathon presentation.

## 60-second explanation

> Bifrost already has fixed request and token limits. Those are useful, but they may miss a virtual key whose behavior changes while each request stays under the limit. FraudGuard keeps a short, bounded baseline for each resolved virtual key and looks for changes such as a new model, a rapid request sequence, token spikes, or repeated failures. It reports a score and reasons for an operator to review. It is observe-only: it does not block requests or store prompts.

## Optional research mention

Use this only if you want to describe the relationship to your earlier research:

> “This prototype also draws on related behavioral anti-fraud ideas from my Indian Patent Application No. 202521074434, published as IN202521074434 A1, titled ‘AI-money maker: intelligent automated revenue generation system with anti-fraud transaction detection by AI.’ That application describes a broader system. FraudGuard is a separate experimental Bifrost plugin applying related behavior-scoring ideas to AI-gateway requests.”

The supplied document identifies this as a patent application, not proof of a granted patent. Keep the relationship precise: do not say that the filing covers or certifies FraudGuard unless that has been confirmed through the application’s claims and legal status.

## Demo

From the project root:

```sh
cd plugins/fraudguard
go run ./cmd/demo
```

The demo sends synthetic data to the in-memory detector; it does not call Bifrost, OpenAI, Anthropic, or any network service.

1. The first requests establish a normal pattern for one sample virtual key.
2. The next request arrives much sooner than the recent pattern.
3. The output includes `rapid_sequence`, with a score of 30.
4. The configured alert threshold is 50, so this finding is shown but is not an alert.
5. The request is not blocked.

Run checks before presenting:

```sh
go test ./...
go test -race ./...
go vet ./...
```

## Likely judge questions

**What problem does this solve?**

It looks for unusual changes in how a virtual key is used, not just how many requests it sends.

**What did you find in Bifrost's code?**

At the pinned commit, Bifrost has request/token limits and provider-failure key rotation. The source inspection found no dedicated plugin that scores changes against a virtual key's rolling behavior history.

**Does FraudGuard block a request?**

No. It emits a score and named reasons. An operator can review the finding; the prototype never rejects or changes the request.

**Does it read prompts, raw keys, or IP headers?**

No. It uses Bifrost's resolved internal virtual-key ID for the per-key profile, hashes IDs for internal state, and does not retain prompts or payloads. It does not use IP headers as identity.

**How does the optional cross-key signal work?**

It is off by default. It can group keys only when Bifrost supplies a resolved authenticated user identity. That identity is HMACed; if Bifrost does not provide it, there is no cross-key signal.

**Why is there no Redis?**

The prototype is intentionally in-memory and single-process, with no network dependency. Shared storage is a possible later design if multi-instance operation becomes a real requirement.

**Is it built into Bifrost's HTTP server?**

Not yet. It implements Bifrost's LLM plugin interface and can be supplied directly through `core.Init`. The HTTP server's built-in plugin loader would need a separate upstream integration.

**Is it production-ready?**

No. It is a tested prototype. It has not been validated against production traffic, benchmarked for a latency guarantee, or made multi-replica.

**Is there a dashboard or business model?**

No. Neither is part of the current community prototype.

## Future community work

1. If maintainers want a built-in HTTP-server feature, add registration, configuration, and server-level integration tests upstream.
2. Review false positives using permissioned, redacted traffic metadata. Do not use prompts, credentials, or private logs.
3. Consider a dashboard only if operators need one; show structured findings and aggregate metrics, not prompts or secrets.
4. Consider shared state only if a real multi-replica requirement appears. Do not promise Redis, latency, or pricing before implementation and measurement.

## Claims to avoid

- Do not say FraudGuard is already installed, accepted, or merged into upstream Bifrost.
- Do not say it blocks suspicious requests, uses Redis, or meets a sub-30ms latency target.
- Do not describe the synthetic demo as live provider traffic.
- Do not present a dashboard, commercial deployment, pricing, or revenue as completed work.
