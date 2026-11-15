# Brief ↔ FraudGuard Event Sync

## Reply for Brief

> Thanks. Brief's local API checkout now sends FraudGuard v2 events from the shared AI-summary path, so both its normal Ollama/Qwen provider and its optional Bifrost provider are covered. Tavily search sends separate metadata-only events.
>
> Events are sent asynchronously through a bounded, fail-open queue. Account references are HMAC-pseudonymized. Prompts, questions, search queries, URLs, source text, PINs, and provider credentials are excluded. `/api/healthz` reports whether delivery is enabled and the pending and dropped event counts.
>
> We ran a loopback-only end-to-end trial with synthetic provider responses and synthetic Tavily results. The FraudGuard v2 receiver accepted all nine events with HTTP 202 and zero drops. No live Ollama, Bifrost, or Tavily request was made. The sender remains disabled by default; no public/non-production endpoint is deployed, and no live events have been sent.
>
> Start and citation/guardrail events are acknowledged but not scored or retained by the current receiver. Tavily duration and result counts are accepted but are not scoring signals. Please keep `FRAUDGUARD_ENABLED` off until a non-production endpoint and server-side credentials are configured; then we can repeat the test against that endpoint and confirm Brief's user-facing response is unchanged.

## Implementation and verification notes

- Brief's shared AI summary function emits start, completion, failure, cancellation, and citation/guardrail outcomes for Ollama and optional Bifrost.
- Brief's Tavily search function emits lifecycle events, duration, result count, and the safe response-envelope validation outcome. It sends no query, URL, or result content.
- The sender uses `FRAUDGUARD_ENABLED=false` by default, requires a server-side client key and a separate HMAC key, accepts HTTPS endpoints (or loopback HTTP in development), and never makes delivery part of the AI response path.
- Local checks: 47 AI/event tests, 7 briefing tests, 9 SAP tests, API typecheck and build, and the FraudGuard Go module test suite passed.
- Local receiver trial: nine synthetic events from Ollama, Bifrost, and Tavily hooks returned HTTP 202; pending and dropped counts ended at zero.
- The Brief checkout changes are local only and have not been committed or pushed. The trial did not use customer data or contact a live AI/search provider.
