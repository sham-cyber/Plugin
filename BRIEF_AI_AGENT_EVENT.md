# Brief ↔ FraudGuard Event Handoff

## Correct architecture

Brief normally generates Qwen 3:0.6b summaries through Ollama. Bifrost is an
optional summary provider, not a required hop. FraudGuard event reporting is
attached to Brief's shared summary flow, so it can observe either provider.
Tavily search is a separate direct retrieval path and emits its own minimized
events.

This is an event feed, not a proxy. Brief sends metadata to FraudGuard's
standalone `POST /api/v2/events` endpoint; the endpoint does not inspect prompts,
search queries, URLs, source text, PINs, or provider credentials, and it cannot
block or change Brief's response.

## Current local implementation

- Model events cover start, completion, failure, cancellation, and citation /
  guardrail outcomes for both Ollama and optional Bifrost summaries.
- Tavily events cover lifecycle, duration, result count, and safe response
  envelope validation. Queries, URLs, and result contents are excluded.
- Account references are HMAC-pseudonymized. A bounded background queue sends
  events asynchronously and fail-open; `/api/healthz` reports enabled state,
  pending events, and dropped events.
- Delivery is disabled by default. `FRAUDGUARD_ENABLED=true`, a full
  `FRAUDGUARD_API_URL`, `FRAUDGUARD_CLIENT_KEY`, and a separate
  `FRAUDGUARD_PROFILE_HMAC_KEY` are required in server-side Secrets.
- A synthetic loopback trial sent nine events from the Ollama, Bifrost, and
  Tavily hooks to the local receiver. All were accepted with HTTP 202 and zero
  drops. No external model or Tavily request was made.
- The Brief changes are local and unpushed. There is no deployed
  non-production FraudGuard endpoint or live event flow.

## Receiver limitations

The current receiver scores terminal model and Tavily events. Start and
citation/guardrail events are acknowledged but not scored or retained.
Tavily duration and result counts are accepted but are not scoring signals.
Do not describe these as full lifecycle history or Tavily anomaly detection.

## Safe next live test

1. Deploy a non-production FraudGuard receiver with HTTPS and an independent
   client key.
2. Put the endpoint, client key, and HMAC key in Brief's server-side Secrets.
3. Keep delivery disabled until those settings are reviewed; then enable it
   only in the non-production environment.
4. Send synthetic Brief requests through Ollama and optional Bifrost, plus a
   synthetic Tavily search. Confirm accepted events, zero unexpected drops,
   and unchanged Brief responses.
5. Keep GitHub Actions unchanged and do not push until the repository owner
   explicitly requests it.
