---
name: FraudGuard production hardening
description: Current production-hardening scope for the Bifrost module while preserving observe-only behavior.
---

**Rule:** For the hackathon, the primary deliverable is a ready-to-integrate FraudGuard plugin for Bifrost. The AshaRatna company dashboard and company alert delivery are secondary. Preserve observe-only request and payment handling: no automatic blocking or request throttles. Optional prompt checks must remain opt-in, scan bounded user text transiently, and never retain or emit prompt content. Finding callbacks should be bounded and asynchronous. Redis is allowed for optional usage/audit product services, not as a behavioral profile store.

**Why:** The user clarified that Bifrost integration is the hackathon goal; AshaRatna's company page and alerts are secondary. The user also explicitly rejected blocking behavior and distinguishes behavioral findings from fixed-rate enforcement. Product metering/audit storage is a separate concern from the detector's per-process behavioral baseline.

**How to apply:** Prioritize a tested, documented Bifrost integration path. Do not describe the module as ready for Bifrost's standard server loader unless verified there. Keep AshaRatna dashboard/alerts separate from the primary integration milestone. Keep scores and optional AI advisory, accept payment/key signals only from trusted host sources, and describe the work as production-oriented but not production-validated until the exact gateway server, operational integrations, state strategy, and workload checks have been verified.
