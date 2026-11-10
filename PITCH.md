# FraudGuard — Hackathon Pitch

## The problem

Fixed request-count limits measure volume, but can miss a key that changes how it behaves while staying below those limits—for example, probing new models, changing token volume, or producing repeated errors.

## The solution

FraudGuard is a separate Go plugin module modeled on the pinned Bifrost plugin contract. It maintains bounded per-key history and reports explainable, observe-only findings. Requests pass through unchanged. Optional payment, credential-status, prompt, and AI-review signals require trusted host inputs and are not automatically discovered.

## What this project demonstrates

- Deterministic behavioral scores and named signal reasons.
- Bounded product usage metering, plan entitlements, signed audit records, and optional signed webhook alerts.
- Near-limit and over-limit notifications are advisory; they never pause or reject requests.
- An authenticated, host-mounted usage export for a company service to pull.
- An aggregate-only admin endpoint at `GET /admin/fraudguard/monetization`.
- A synthetic AshaRatna cost-tracker view with public-rate citations, clearly separated from live customer data.

This is a working hackathon module, not a built-in Bifrost feature or a validated production deployment. No upstream acceptance, company pilot, billing connection, or realized savings are claimed.

## Business and money flow

The Apache-2.0 plugin core is free to review. AshaRatna Tech could offer the company-facing reporting, cost analysis, integrations, and support as a paid service. A Bifrost distribution or revenue-share route would require a separate agreement.

Bifrost exports usage and risk aggregates; AshaRatna owns the commercial rate card, cost estimates, subscription billing, and financial reporting. The prototype has no actual provider invoice feed or FraudGuard price. The displayed `$0.78` is a synthetic example—not profit, an invoice, or savings from blocking.

## Configure the cost view for a future live source

The company page is synthetic by default. To attach a live cost feed, configure the frontend build variable:

```text
VITE_FRAUDGUARD_COST_API_PATH=/api/asharatna/fraudguard/cost-summary
```

The configured path must be a same-origin `/api/` route backed by an authenticated AshaRatna server. The browser sends the existing session cookie; it must not contain provider keys or billing secrets. The server must authenticate the company, resolve its tenant from trusted session context, and pull the tenant-scoped Bifrost export.

The route must return `asharatna.cost.v1` with `authenticated: true`, `tenant_scoped: true`, a complete model breakdown, the period, token/request totals, an estimated cost, and the rate-card source/date. The page polls the configured route and displays a validated live summary when available. If the source is missing, unauthorized, unavailable, or incomplete, the cost card shows no number and does **not** fall back to synthetic costs. Other finding cards remain synthetic until a separate authenticated findings feed is built.

That AshaRatna API route and company login are **not mounted in this prototype**. Until they exist, leave the variable unset to show the labeled synthetic demo.

## Judge verification

Run from the repository root:

```sh
cd plugins/fraudguard
GOSUMDB=sum.golang.org go test -count=1 ./...
GOSUMDB=sum.golang.org go test -race -count=1 ./...
GOSUMDB=sum.golang.org go vet ./...
GOSUMDB=sum.golang.org go run ./cmd/demo
```

The demo uses synthetic events and makes no provider or payment calls. To inspect the admin monetization snapshot, the Bifrost host must mount `NewAdminHandler` on a private listener and provide its authorization callback; do not expose it publicly. The snapshot reports plugin counters and budget-monitor totals. With a trusted host callback, it can include aggregate tier counts; without one, it labels the process-cache-only distribution. It does not report blocked requests, actual invoices, revenue, or profit.

## Current boundaries

- Requests are never blocked, throttled, or rewritten.
- Tier and overage updates come from trusted host services; this module does not accept subscription claims from request headers.
- A host must verify billing webhooks and apply the plan update before calling the tier refresh helper.
- Email/SMS delivery, external billing-provider calls, the AshaRatna live API, company authentication, and production multi-replica validation are not included.
