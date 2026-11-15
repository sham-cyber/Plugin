---
name: Brief event scope
description: The intended Brief-to-FraudGuard event path across its default and optional AI providers.
---

Brief's primary summary setup is Qwen 3:0.6b through Ollama; Bifrost is an optional provider. FraudGuard event reporting should cover the shared summary path regardless of provider and include Tavily retrieval metadata directly.

**Why:** the user explicitly corrected the assumption that Bifrost is Brief's only AI path and asked that whichever configured AI provider is used can report events to FraudGuard.

**How to apply:** when changing the Brief bridge, instrument the shared summary path for both Ollama and optional Bifrost, and the Tavily path separately. Do not imply that Brief requires Bifrost to send events.
