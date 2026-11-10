# FraudGuard Monetization Architecture

## Status

This is a proposed business model for a Bifrost-compatible FraudGuard companion module. It is not a published price list, signed partnership, revenue forecast, or active billing integration. The current plugin remains observe-only and does not block or throttle requests.

## Customer and distribution

The target customers are organizations and AI gateway operators that want to see unusual changes in their own AI usage across teams, applications, and virtual keys. One gateway deployment can serve many keys; the business model should not assume a separate deployment cost for every key.

Two possible commercial routes are:

1. **[AshaRatna Tech](https://asharatnatech.com) sells FraudGuard directly** to gateway operators as a subscription or self-hosted license.
2. **A gateway vendor distributes or bundles it** under a separately negotiated marketplace or revenue-share agreement.

Neither a Bifrost partnership nor a marketplace listing is currently established. FraudGuard is not a built-in upstream Bifrost feature.

## Proposed plan structure

The code recognizes `Free`, `Pro`, and `Enterprise` plan tiers. Operators supply the policy catalog and tenant-to-plan assignments through trusted server-side configuration. These are capability tiers, not published prices.

| Proposed tier | Commercial role | Possible entitlements |
| --- | --- | --- |
| Free | Evaluation and small self-hosted pilots | Core behavioral findings, bounded local state, optional basic usage export |
| Pro | Paid team or gateway subscription | Configured usage allowance, additional advisory signals, usage export, and webhook alerts |
| Enterprise | Larger organizations and gateway operators | Negotiated deployment, support, privacy, retention, and integration requirements |

Actual prices, volume allowances, support commitments, and service levels remain to be validated with buyers. Current monthly limits create warnings and observe-only overage records; they do not cut off service. Tier entitlements may enable or disable signals and exports, but do not block high-risk requests.

## Overage notifications and tier changes

Near-limit and over-limit usage can produce an advisory audit event and an optional signed webhook alert when the tenant's plan enables webhook alerts. Notifications are deduplicated by tenant, month, tier, and threshold stage. Redis-backed metering uses an atomic claim so replicas share deduplication; without Redis, the fallback is bounded and process-local. Delivery failure releases the claim for a later retry. No request is throttled, paused, or rejected.

For subscription changes, the host must first authenticate the billing event and update its trusted plan source. It can then call `TierSyncManager.RefreshAfterTrustedUpdate` to bypass the cache and refresh the tenant policy immediately. FraudGuard does not accept tier values from inference headers or independently consume billing webhooks.

## Metering and export architecture

The current product ledger records aggregate request counts, prompt and completion tokens, detector checks, detector latency, findings, and alerts. Tenant references are HMAC digests. Usage events are deduplicated by their event ID and contain no prompts, responses, provider API keys, or customer-record content.

`BuildUsageExport` and `ExportUsageJSON` provide a versioned monthly summary. `NewBillingExportHandler` adds a host-mountable, pull-based endpoint:

```text
GET /admin/fraudguard/usage-export?month=YYYY-MM
```

The host must provide authorization and resolve the tenant from authenticated server-side context. The endpoint rejects tenant selectors in the request, returns no-store JSON, and applies the configured usage-export entitlement when plan services are supplied. Mount it only on a private, authorized listener.

This endpoint exports usage data; it does not call Stripe or another billing provider, store billing credentials, set prices, issue invoices, or charge a payment method. A host billing service may consume the export and reconcile it with its authoritative provider usage and rate card.

The private `GET /admin/fraudguard/monetization` endpoint reports aggregate plugin and budget-monitor counters. A host may supply a trusted aggregate tier-count resolver through `NewAdminHandlerWithOptions`; without it, the reported distribution is limited to the process-local policy cache. This endpoint does not expose tenant identities, blocked-request counts, actual invoices, revenue, or profit.

## Chosen AshaRatna integration

The selected direction is **AshaRatna pulls from Bifrost**. AshaRatna should call the host-mounted `GET /admin/fraudguard/usage-export?month=YYYY-MM` endpoint over a private authenticated connection. The Bifrost host authenticates the company and resolves its tenant; callers cannot choose another tenant by supplying an ID. Provider credentials, raw virtual keys, and prompts do not leave Bifrost.

The Go module provides the authenticated, amount-free `fraudguard.usage.v2` export, including bounded provider/model token aggregates and a `model_breakdown_complete` flag. The Bifrost host must mount it and configure authorization and tenant resolution. AshaRatna can apply its own dated rate card to complete model rows; it must not calculate a full estimate when the completeness flag is false. The company dashboard is not connected to a live Bifrost instance.

The AshaRatna-facing cost panel is synthetic by default. To attach a future live cost source, set `VITE_FRAUDGUARD_COST_API_PATH` to a same-origin `/api/` route served by the authenticated AshaRatna backend. The page polls that route and switches the cost panel to live only when it returns the expected `asharatna.cost.v1` schema, an authenticated tenant scope, and a complete model breakdown. If the configured route is unauthorized, unavailable, or incomplete, it shows no cost number rather than falling back to synthetic values. The AshaRatna route and company login are not implemented in this prototype; behavior/payment findings remain synthetic even if a live cost source is later attached.

## Revenue and savings measurement

The dashboard now demonstrates a **counterfactual cost proxy** using real public list prices and a synthetic provider/model mix. It models four synthetic AI requests flagged by a separate usage scenario at **$0.78 USD**, assuming they could have been stopped before provider submission. Synthetic payment events are excluded. No requests were blocked, so this is not realized savings or a claim that FraudGuard saved money. The synthetic token counts, provider mix, and outcome are not customer data.

The sample uses standard public text-token prices checked on November 10, 2026: OpenAI GPT-5.4 mini at $0.75 input / $4.50 output per million tokens; Anthropic Claude Sonnet 5.5 at $2 / $10; and Google Gemini 3.8 Flash at $0.75 / $3.75 through December 31, 2026. The Google rate card lists a change to $1.50 / $7.50 from January 1, 2027. The estimate excludes caching, long-context tiers, tools, grounding, batch discounts, regional uplifts, credits, negotiated discounts, taxes, and provider-specific billing adjustments. These are reference prices, not invoices.

Official rate sources:

- [OpenAI API pricing](https://developers.openai.com/api/docs/pricing)
- [Anthropic list prices effective September 28, 2026](https://www-cdn.anthropic.com/files/4zrzovbb/website/f237fd89990be52c0f683eb5c9d19bc17667f10c.pdf)
- [Gemini Developer API pricing](https://ai.google.dev/gemini-api/docs/pricing)

Potential gross recurring revenue can be modeled once pricing and buyer counts are validated:

```text
monthly gross revenue =
  (active Pro customers × Pro monthly price)
  + (active Enterprise customers × Enterprise monthly price)
  + metered overage revenue, if adopted

net contribution =
  gross revenue
  - hosting and storage
  - payment processing
  - support and operations
  - sales, legal, tax, and partner costs
```

No customer count, conversion rate, churn, price, or profit figure is asserted here. Validate willingness to pay, the preferred billing unit (organization, gateway deployment, protected usage, or a mix), and cost to serve before publishing a forecast.

## Current capabilities versus work still needed

**Implemented:** HMAC-based tenant references, monthly provider/model usage aggregation (bounded to 64 model combinations per tenant-month), optional plan entitlements, deduplicated advisory near/over-limit notifications, trusted immediate tier-cache refresh, versioned JSON usage export, a host-authorized export handler, an aggregate-only monetization stats endpoint, and a company-page counterfactual cost demo using cited public prices.

**Not implemented:** the authenticated AshaRatna cost-summary route and company login, live AshaRatna-to-Bifrost data exchange, production host mounting, automated provider billing, invoices, payment collection, a commercial FraudGuard price catalog, a managed multi-tenant SaaS service, actual request blocking, or a Bifrost marketplace/revenue-share agreement.

Before charging customers, an operator must add and test its own billing-provider integration, authentication and tenant mapping, reconciliation and retry policy, customer consent, data-retention policy, support process, and commercial terms. FraudGuard findings must remain advisory unless a separate, reviewed product decision explicitly changes the observe-only scope.
