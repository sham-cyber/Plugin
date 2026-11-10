---
name: FraudGuard production hardening
description: Current production-hardening scope for the Bifrost module while preserving observe-only behavior.
---

**Rule:** The user wants production-oriented hardening while preserving observe-only request and payment handling: no automatic blocking or request throttles. Optional prompt checks must remain opt-in, scan bounded user text transiently, and never retain or emit prompt content. Finding callbacks should be bounded and asynchronous. Redis is allowed for optional usage/audit product services, not as a behavioral profile store.

**Why:** The user explicitly rejected blocking behavior and distinguishes behavioral findings from fixed-rate enforcement. Product metering/audit storage is a separate concern from the detector's per-process behavioral baseline.

**How to apply:** Keep scores and optional AI advisory, accept payment/key signals only from trusted host sources, and describe the work as production-oriented but not production-validated until the exact gateway server, operational integrations, state strategy, and workload checks have been verified.
