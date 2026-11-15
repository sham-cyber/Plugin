---
name: FraudGuard host-model review
description: Product decision for the optional AI review path in the Bifrost plugin.
---

Optional AI review should use the host's configured Bifrost model rather than a separate provider client. Deterministic Go signals remain authoritative; AI review stays off by default, asynchronous, and advisory, and receives only sanitized finding metadata.

**Why:** the user selected the Bifrost host model for AI review.

**How to apply:** do not send prompts, API keys, or customer identifiers to the model, and never let its output change scores or request handling.
