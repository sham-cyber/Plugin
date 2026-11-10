# FraudGuard Release Manifest

**Status:** Hackathon source release candidate; not a production deployment.

## Included source

| Area | Path | Purpose |
|---|---|---|
| Bifrost plugin module | `plugins/fraudguard/` | Observe-only Go detector implementing Bifrost's `schemas.LLMPlugin` contract |
| Plugin integration checks | `plugins/fraudguard/main_test.go`, `plugins/fraudguard/sanity_test.go` | Core hook and product-service sanity tests |
| Optional product services | `plugins/fraudguard/metering.go`, `plugins/fraudguard/product.go`, `plugins/fraudguard/overage_notifier.go`, `plugins/fraudguard/tier_sync.go` | Usage ledger, tier policy, advisory notices, and host-triggered refresh |
| Admin summaries | `plugins/fraudguard/admin.go`, `plugins/fraudguard/monetization_stats.go` | Host-authorized, aggregate-only status and monetization data |
| Local release helper | `scripts/deploy.sh` | Local preflight, source packaging, optional Compose demo, and opt-in local tag creation; production deploy refuses |
| Company cost demo | `artifacts/fraudguard-bifrost-page/` | Synthetic-by-default dashboard with a validated future live-source adapter |
| User documentation | `README.md`, `MONETIZATION.md`, `PITCH.md`, `PRESENTATION.md`, `RUNBOOK.md` | Setup, business flow, demo outline, and operating limits |
| Verification record | `VERIFICATION_REPORT.md` | Commands run and checks not performed |

## Verified locally

Run from the repository root:

```sh
make fraudguard-sanity
make fraudguard-verify
```

`fraudguard-sanity` uses miniredis and a synthetic request to check Redis fixture connectivity, trusted tier loading, prompt-risk scoring, pass-through behavior, and amount-free usage export. It is not a live Redis, production HTTP gateway, or provider test.

## Release boundaries

- The module is separate from Bifrost's built-in plugin loader; the host must supply it through the documented core initialization path.
- Requests and payments are never blocked, throttled, or rewritten.
- The local dashboard remains synthetic unless an authenticated, tenant-scoped AshaRatna source is configured and returns valid data. The AshaRatna route and company login are not included.
- AshaRatna/Bifrost billing, real provider invoices, pricing, realized savings, and revenue are not connected or claimed.
- No production deployment target is configured. `bash scripts/deploy.sh deploy` refuses safely; `preflight`, `package`, and `tag` are local-only operations.
- The GitHub Actions workflow is intentionally pending and is not part of the push while CI remains pending. No CI result or production release is claimed.
