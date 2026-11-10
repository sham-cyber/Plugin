---
name: FraudGuard–AshaRatna data flow
description: Chosen direction for company usage exports and cost estimates.
---

**Rule:** AshaRatna consumes tenant-scoped, authenticated pull exports from Bifrost; Bifrost remains the source of truth. Exports may include HMAC tenant references and bounded provider/model usage totals, but never raw virtual keys, prompts, or provider credentials. Calculate cost estimates outside the detector from dated public rates and label them counterfactual while the product remains observe-only.

**Why:** The user chose company-side usage visibility without moving provider credentials or trusting caller-supplied tenant IDs. The plugin already has a host-authorized pull-export pattern.

**How to apply:** Default to labeled synthetic examples. Show live cost data only after a configured same-origin AshaRatna source returns an authenticated, tenant-scoped, complete provider/model summary; if that source fails or is incomplete, show no cost number and never silently fall back to synthetic. Keep findings synthetic until a separate live findings source exists. Include the commercial revenue/cost model in the project overview and label proxy figures synthetic, not actual savings, revenue, or profit.
