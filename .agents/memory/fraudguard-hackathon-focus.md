---
name: FraudGuard hackathon focus
description: Project scope rule to keep presentation and proposed work tied to the working prototype.
---

**Rule:** For this FraudGuard hackathon project, focus first on the Bifrost Go prototype and the theme “Bring your ideas, experiment with cutting-edge AI technology.” Optional AI analysis is approved through a Bifrost-host-supplied model adapter. Keep it opt-in and advisory, pass only sanitized behavioral summaries, and preserve deterministic scoring and observe-only request handling. Do not claim a live model integration until the host supplies and tests the adapter.

**Why:** the user said the project must follow the hackathon theme and selected optional AI analysis while preserving accurate claims about implementation.

**How to apply:** Keep proposed work tied to demonstrating, testing, or explaining the Bifrost Go prototype. Distinguish the AI-gateway context from the rule-based detector, preserve verified claims, and label synthetic UI accurately. Do not add a provider-specific model client or ask for provider credentials inside FraudGuard; the Bifrost host owns model configuration.
