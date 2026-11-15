# FraudGuard Host Event API v2

This is the exact contract implemented by the FraudGuard receiver at
`plugins/fraudguard/httpapi/events.go`. It is provided for the Brief integration.
It does not mean the API is deployed or that the Brief bridge is enabled.

## Endpoint

- Method: `POST`
- Path: `/api/v2/events`
- Authentication: `Authorization: Bearer <FraudGuard client key>`
- Content type: `application/json`
- Success status: `202 Accepted`
- Enforcement: observe-only; requests are never blocked or changed
- Legacy route: `/api/v1/score` remains unchanged

The API key belongs in Brief's server-side Secrets only. Do not put it in
browser code, source files, event bodies, or logs.

## Common fields

Every event requires these fields:

| Field | Type | Rule |
|---|---|---|
| `event_id` | string | Unique event identifier; 1–128 characters matching `[A-Za-z0-9][A-Za-z0-9._:-]*` |
| `attempt_id` | string | Opaque ID shared by events from one attempt; same character/length rule |
| `profile_ref` | string | Stable opaque pseudonym; same character/length rule; not an email, account name, API key, or other identifying value |
| `operation` | string | `model_summary` or `tavily_search` |
| `event_type` | string | `start`, `completion`, `failure`, `cancellation`, or `citation_guardrail` |
| `timestamp` | string | RFC 3339 timestamp; at most 24 hours old and no more than 2 minutes in the future |
| `status` | string | Must match the `event_type` mapping below |

`event_id` identifies one event; `attempt_id` relates events for one attempt.
Each event in an attempt must have its own `event_id`.

## Event type and status mapping

| `event_type` | Required `status` |
|---|---|
| `start` | `started` |
| `completion` | `completed` |
| `failure` | `failed` |
| `cancellation` | `cancelled` |
| `citation_guardrail` | `passed`, `failed`, or `not_run` |

For `citation_guardrail`, also send `validation_outcome` with the same value as
`status`. Other event types must omit `validation_outcome`.

## Model-summary fields

For `operation: "model_summary"`:

- `provider` and `model` are required. Each is 1–64 characters and must match
  `[A-Za-z0-9][A-Za-z0-9._/-]*`.
- Terminal events (`completion`, `failure`, `cancellation`) require
  `duration_ms` from 0 through 600,000.
- Terminal events require `token_count_source`:
  - `provider-reported`: both `input_tokens` and `output_tokens` are required.
  - `estimated`: both token counts are required.
  - `unavailable`: omit both token counts.
- Token counts must each be integers from 0 through 100,000,000.
- `result_count` is not allowed.
- For `start` and `citation_guardrail`, omit token counts. The token source may
  be omitted or set to `unavailable`.

The receiver accepts token-count provenance but the current scoring engine
does not use that provenance to change a score.

### Completed model event example

```json
{
  "event_id": "evt-model-001",
  "attempt_id": "attempt-model-001",
  "profile_ref": "opaque-profile-7",
  "operation": "model_summary",
  "event_type": "completion",
  "timestamp": "2026-11-15T11:00:00Z",
  "status": "completed",
  "provider": "ollama",
  "model": "qwen3-0.6b",
  "input_tokens": 120,
  "output_tokens": 64,
  "token_count_source": "provider-reported",
  "duration_ms": 245
}
```

## Tavily search fields

For `operation: "tavily_search"`:

- `provider` must be exactly `tavily`.
- Omit `model`, token counts, and token-count source. The source may instead be
  `unavailable`, but counts must still be omitted.
- A `completion` requires `duration_ms` and `result_count`.
- `failure` and `cancellation` require `duration_ms`; `result_count` is optional.
- `start` and `citation_guardrail` may include duration/result count when known.
- `duration_ms` must be from 0 through 600,000; `result_count` must be from 0
  through 1,000.
- `citation_guardrail` requires `validation_outcome` and matching `status`.

Never send the Tavily query, URLs, source snippets, or result content.

### Completed Tavily event example

```json
{
  "event_id": "evt-search-001",
  "attempt_id": "attempt-search-001",
  "profile_ref": "opaque-profile-7",
  "operation": "tavily_search",
  "event_type": "completion",
  "timestamp": "2026-11-15T11:00:00Z",
  "status": "completed",
  "provider": "tavily",
  "duration_ms": 245,
  "result_count": 4
}
```

## Response

A scored terminal event returns HTTP `202` with this shape:

```json
{
  "schema_version": "fraudguard.event.v2",
  "event_id": "evt-model-001",
  "attempt_id": "attempt-model-001",
  "accepted": true,
  "operation": "model_summary",
  "event_type": "completion",
  "status": "completed",
  "scored": true,
  "score": {
    "schema_version": "fraudguard.score.v1",
    "event_id": "evt-model-001",
    "risk_score": 0,
    "threshold": 60,
    "alert": false,
    "warmup": true,
    "signals": [],
    "enforcement": "observe_only"
  },
  "enforcement": "observe_only"
}
```

The score is deterministic and advisory. `risk_score` and `threshold` are
0–100. Each signal contains `name`, `reason`, and `weight`. A warmup or benign
event can have an empty signal list.

`start` and `citation_guardrail` events return `202`, `accepted: true`, and
`scored: false`; they do not include `score`. The receiver does not retain them
as event history or correlate an `attempt_id` across separate calls. Terminal
model events are scored using provider/model, token counts, frequency, and
failure signals. Terminal Tavily events are scored for provider/frequency and
failure signals. Tavily duration and result counts are validated but are not
yet scoring signals.

## Idempotency and resource limits

- The authenticated tenant is derived from the bearer key; `profile_ref` is
  scoped to that tenant.
- The per-tenant request limit is shared with `/api/v1/score` (default 120 per
  minute; configurable from 1 to 100,000).
- Request bodies are limited to 4 KiB and must contain exactly one JSON object.
- Unknown fields are rejected. This intentionally rejects prompts, source
  text, search queries, URLs, PINs, credentials, and arbitrary extra fields.
- Idempotency is keyed by authenticated tenant plus `event_id`, retained in
  process memory for 10 minutes, and bounded to 20,000 entries.
- Repeating the same ID and same event returns the cached result and
  `deduplicated: true`. Reusing an ID with different event content returns
  `409 {"error":"event_id_conflict"}`.
- Idempotency data and behavior profiles reset when the API process restarts.

Other error statuses include `400` for malformed/invalid JSON or fields,
`401` for missing/invalid bearer credentials, `405` for a non-POST method,
`413` for an oversized body, `415` for non-JSON content, `429` for the per-tenant
request limit, `503` if the bounded idempotency store is full, and `408` if the
request is cancelled while waiting on a duplicate event.

## Host requirements and current integration status

Brief should emit these events from server-side code using a bounded
background queue. Delivery must be best-effort and fail-open so the AI response
does not wait for FraudGuard. Brief must expose its own queue-drop count:
FraudGuard cannot count events that never arrive.

The receiver implementation and fixtures are in this repository, but the
service is not deployed or live-tested. The Brief-side bridge remains local and
disabled; it still needs to use this v2 contract and add Tavily event emission.
Do not treat this file as confirmation that production credentials, a public
endpoint, or an end-to-end integration are ready.

## Test fixtures

- Model completion: `plugins/fraudguard/httpapi/testdata/model_completion.json`
- Tavily completion: `plugins/fraudguard/httpapi/testdata/tavily_completion.json`

The fixtures use `__TEST_TIMESTAMP__` as a test-only placeholder. Replace it
with a current RFC 3339 timestamp when making a manual request.
