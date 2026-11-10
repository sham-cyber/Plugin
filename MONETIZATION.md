# FraudGuard Monetization Architecture

## Status

This is a proposed business model for a Bifrost-compatible FraudGuard companion module. It is not a published price list, signed partnership, revenue forecast, or active billing integration. The current plugin remains observe-only and does not block or throttle requests.

## Customer and distribution

The target customers are organizations and AI gateway operators that want to see unusual changes in their own AI usage across teams, applications, and virtual keys. One gateway deployment can serve many keys; the business model should not assume a separate deployment cost for every key.

Two possible commercial routes are:

1. **AshaRatna Tech sells FraudGuard directly** to gateway operators as a subscription or self-hosted license.
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

## Metering and export architecture

The current product ledger records aggregate request counts, prompt and completion tokens, detector checks, detector latency, findings, and alerts. Tenant references are HMAC digests. Usage events are deduplicated by their event ID and contain no prompts, responses, provider API keys, or customer-record content.

`BuildUsageExport` and `ExportUsageJSON` provide a versioned monthly summary. `NewBillingExportHandler` adds a host-mountable, pull-based endpoint:

```text
GET /admin/fraudguard/usage-export?month=YYYY-MM
```

The host must provide authorization and resolve the tenant from authenticated server-side context. The endpoint rejects tenant selectors in the request, returns no-store JSON, and applies the configured usage-export entitlement when plan services are supplied. Mount it only on a private, authorized listener.

This endpoint exports usage data; it does not call Stripe or another billing provider, store billing credentials, set prices, issue invoices, or charge a payment method. A host billing service may consume the export and reconcile it with its authoritative provider usage and rate card.

## Revenue and savings measurement

FraudGuard can report measured usage volume and finding/alert counts. It cannot currently calculate money saved by blocking fraud: the MVP never blocks requests, has no verified counterfactual token count, and does not contain authoritative per-model provider prices. Any “saved amount” shown today would be an unsupported estimate.

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

**Implemented:** HMAC-based tenant references, monthly usage aggregation, optional plan entitlements, observe-only warnings, JSON usage export, and a host-authorized export handler.

**Not implemented:** automated provider billing, invoices, payment collection, a commercial price catalog, a managed multi-tenant SaaS service, savings claims from blocked traffic, or a Bifrost marketplace/revenue-share agreement.

Before charging customers, an operator must add and test its own billing-provider integration, authentication and tenant mapping, reconciliation and retry policy, customer consent, data-retention policy, support process, and commercial terms. FraudGuard findings must remain advisory unless a separate, reviewed product decision explicitly changes the observe-only scope.
