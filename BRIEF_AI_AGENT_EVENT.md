# Brief AI Agent Handoff: Use Workspace AI Through Bifrost

**Purpose:** Give the Brief project agent a clear task to verify and finish the live Brief → Bifrost → FraudGuard path, without adding provider keys to source control.

## Current status

- The Brief repository already has an optional Bifrost route for Qwen summaries. It was published in [this Brief commit](https://github.com/sham-cyber/Brief/commit/692cb99fc276bd689543bae2636ff55dc0c0c84b).
- The route sends summaries to Bifrost's OpenAI-compatible `/v1/chat/completions` endpoint with the server-side `x-bf-vk` virtual-key header.
- Tavily search remains direct. Ollama remains the default unless the Brief server is configured for Bifrost.
- No live Bifrost address or virtual key has been configured or tested yet.
- The owner reports that the Replit workspace has working AI, but the Brief repository does not contain AI provider keys.

## Important distinction

`BIFROST_VIRTUAL_KEY` is a key that authorizes Brief to use a Bifrost gateway. It is **not** an AI provider API key. Bifrost must already be configured to reach the model. A local Qwen/Ollama model may not need a provider API key.

Do not assume that an AI model available to the Replit Agent is automatically callable by the Brief API server. First check whether the workspace's configured AI integration is available to server-side Brief code. Keep all credentials in Replit Secrets or the approved integration flow; never put secrets in source, logs, screenshots, or Git.

## Task for the Brief project agent

Inspect the current Brief `main` branch and finish the real runtime configuration for this route:

1. Review the existing Bifrost provider code and tests before editing. Avoid building a duplicate adapter.
2. Identify how the working AI in this Replit workspace is exposed to the **Brief API server**. Confirm whether Bifrost can route to that model. Do not invent an endpoint or assume the Agent's own AI is available to the deployed app.
3. Keep the intended request path: **Brief → Bifrost → FraudGuard → configured model**. Do not send monitored Qwen requests directly to a separate AI service, because that would bypass FraudGuard.
4. If the workspace AI cannot be reached through Bifrost, report the exact blocker and the supported alternative before changing the architecture.
5. Keep Tavily search separate and preserve the current Ollama default. When Bifrost mode is selected, gateway failure must leave the source-based brief available; it must not silently fall back to a direct model call.
6. Configure the Bifrost URL, model name, and virtual key only through server-side environment/secrets. Keep user activation and per-brief opt-in in place.
7. Keep FraudGuard observe-only. Do not block, alter, or delay user requests based on a finding.
8. Add or update tests for configuration, virtual-key header handling, missing gateway configuration, source-only fallback, and request cancellation. Use synthetic test data.
9. Run the Brief API tests and type checks. Run a live test only against an explicitly configured non-production Bifrost gateway; do not claim a live integration test if only mocks ran.
10. Update the Brief README with the actual setup and verification results, then commit and push the verified Brief changes to `sham-cyber/Brief` `main`. Do not add or enable GitHub Actions as part of this task.

## Expected server configuration

Use the actual model ID returned by the configured Bifrost gateway's model list. These are names only; put the virtual key value in the server's secret manager, not this file:

```text
BRIEF_AI_PROVIDER=bifrost
BIFROST_BASE_URL=https://<configured-bifrost-host>
BIFROST_MODEL=<model-id-visible-to-the-virtual-key>
BIFROST_VIRTUAL_KEY=<server-side-bifrost-virtual-key>
```

Before enabling this in a shared deployment, confirm the Bifrost virtual key is restricted to the intended model and that the FraudGuard plugin is loaded in that Bifrost server.

## What to report back

- Whether the workspace AI is actually callable from the Brief API server and routable through Bifrost.
- Which configuration was added, without revealing secret values.
- Exact tests/builds run and whether they used mocks or a live test gateway.
- The Brief commit link after a successful push.
