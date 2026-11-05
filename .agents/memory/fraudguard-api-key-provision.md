---
name: FraudGuard API-key provision
description: User-stated plan to provide API keys for the FraudGuard plugin; credential flow needs clarification.
---

**Rule:** The user said, “we are going to provide api keys for plugin.” Treat API-key provision as part of the intended FraudGuard scope. The type of key and where it should be configured still need clarification.

**Why:** the user asked to keep this in mind.

**How to apply:** Before implementing key input or storage, clarify whether the user means provider credentials configured in Bifrost or a separate FraudGuard credential. Never put real secret values in chat, documents, or source files; use the workspace secrets flow when actual secret configuration is needed.
