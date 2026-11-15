# Brief ↔ FraudGuard Event Contract Sync

Thanks. Agreed—we should include Tavily search/retrieval as well as Ollama
and Bifrost model summaries.

I checked the current FraudGuard `POST /api/v1/score` handler. It strictly
accepts completed model events with `event_id`, `profile_ref`, `provider`,
`model`, `timestamp`, token counts, and `failed`. It cannot currently represent
separate lifecycle events, citation/guardrail outcomes, or Tavily search
events. Sending those fields to the current endpoint would be rejected or
misrepresented.

Before changing Brief, please confirm a versioned event contract for:

- Event/attempt ID, opaque pseudonymous profile reference, operation, event
  type, and timestamp.
- Start, completion, failure, cancellation, and citation/guardrail outcomes.
- Tavily search/retrieval duration, result count, and safe validation outcome.
  Never include the search query, URLs, or source content.
- Model provider/model, status, and token counts explicitly marked as
  provider-reported, estimated, or unavailable.

Keep the existing score endpoint backward-compatible if possible. Keep event
delivery asynchronous and fail-open, and make dropped-event counts observable.
Do not send prompts, source text, PINs, credentials, or identifying account
values. Keep credentials in server-side Secrets.

Once the receiver schema and response are agreed and tested, Brief can add the
Tavily hook and align its model-provider events. Then run a non-production
live test and confirm Brief's response is unchanged. No follow-up code changes
are requested before that agreement.
