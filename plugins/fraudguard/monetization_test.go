// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type billingTestLedger struct {
	tenantID string
	month    string
	calls    int
	totals   UsageTotals
	err      error
}

func (l *billingTestLedger) RecordUsage(context.Context, UsageEvent) error {
	return nil
}

func (l *billingTestLedger) UsageForMonth(_ context.Context, tenantID, month string) (UsageTotals, error) {
	l.calls++
	l.tenantID = tenantID
	l.month = month
	if l.err != nil {
		return UsageTotals{}, l.err
	}
	totals := l.totals
	totals.Month = month
	return totals, nil
}

func TestBillingExportHandlerUsesTrustedTenantAndReturnsAmountFreeUsage(t *testing.T) {
	ledger := &billingTestLedger{totals: UsageTotals{
		TenantRef: "opaque-tenant-digest", Requests: 8, PromptTokens: 120, CompletionTokens: 80,
		FraudChecks: 8, Findings: 1, Alerts: 1,
	}}
	catalog, err := NewTierPolicyCatalog([]TierPolicy{{
		Tier: PlanPro, AlertThreshold: 60,
		Features: FeatureEntitlements{UsageExport: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewBillingExportHandler(BillingExportHandlerOptions{
		Authorize: func(*http.Request) bool { return true },
		ResolveTenant: func(*http.Request) (string, error) {
			return "tenant-from-authenticated-host", nil
		},
		Ledger:  ledger,
		Plans:   testPlanResolver(func(context.Context, string) (PlanTier, error) { return PlanPro, nil }),
		Catalog: catalog,
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, BillingUsageExportPath+"?month=2026-11", nil)
	request.Header.Set("X-Tenant-ID", "caller-controlled-tenant")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("billing usage response must not be cached")
	}
	if ledger.tenantID != "tenant-from-authenticated-host" || ledger.month != "2026-11" {
		t.Fatalf("ledger used untrusted request identity: tenant=%q month=%q", ledger.tenantID, ledger.month)
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{"tenant-from-authenticated-host", "caller-controlled-tenant", "price", "revenue", "blocked"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("export contains unsupported or private data %q: %s", forbidden, body)
		}
	}
	var report UsageExport
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != "fraudguard.usage.v1" || report.TenantRef != "opaque-tenant-digest" ||
		report.Usage.Requests != 8 || report.Plan == nil || report.Plan.Tier != PlanPro {
		t.Fatalf("unexpected billing export: %+v", report)
	}
}

func TestBillingExportHandlerRejectsTenantOverridesAndUnauthorizedCallers(t *testing.T) {
	ledger := &billingTestLedger{totals: UsageTotals{TenantRef: "opaque"}}
	tenantResolutionCalls := 0
	handler, err := NewBillingExportHandler(BillingExportHandlerOptions{
		Authorize: func(r *http.Request) bool { return r.Header.Get("X-Test-Authorized") == "yes" },
		ResolveTenant: func(*http.Request) (string, error) {
			tenantResolutionCalls++
			return "trusted-tenant", nil
		},
		Ledger: ledger,
	})
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRequest(http.MethodGet, BillingUsageExportPath+"?month=2026-11", nil)
	unauthorizedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized || tenantResolutionCalls != 0 {
		t.Fatalf("unauthorized request reached tenant resolver: status=%d calls=%d", unauthorizedRecorder.Code, tenantResolutionCalls)
	}

	override := httptest.NewRequest(http.MethodGet, BillingUsageExportPath+"?month=2026-11&tenant_id=attacker", nil)
	override.Header.Set("X-Test-Authorized", "yes")
	overrideRecorder := httptest.NewRecorder()
	handler.ServeHTTP(overrideRecorder, override)
	if overrideRecorder.Code != http.StatusBadRequest || tenantResolutionCalls != 0 || ledger.calls != 0 {
		t.Fatalf("tenant override was not rejected before lookup: status=%d resolver=%d ledger=%d",
			overrideRecorder.Code, tenantResolutionCalls, ledger.calls)
	}
}

func TestBillingExportHandlerHonorsPlanEntitlementAndKeepsOveragesAdvisory(t *testing.T) {
	ledger := &billingTestLedger{totals: UsageTotals{TenantRef: "opaque", Requests: 2}}
	noExportCatalog, err := NewTierPolicyCatalog([]TierPolicy{{
		Tier: PlanFree, MonthlyRequestLimit: 1, AlertThreshold: 60,
		Features: FeatureEntitlements{UsageExport: false},
	}})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewBillingExportHandler(BillingExportHandlerOptions{
		Authorize: func(*http.Request) bool { return true },
		ResolveTenant: func(*http.Request) (string, error) {
			return "trusted-tenant", nil
		},
		Ledger:  ledger,
		Plans:   testPlanResolver(func(context.Context, string) (PlanTier, error) { return PlanFree, nil }),
		Catalog: noExportCatalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, BillingUsageExportPath+"?month=2026-11", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || ledger.calls != 0 {
		t.Fatalf("non-entitled plan accessed export: status=%d ledger calls=%d", recorder.Code, ledger.calls)
	}

	policy := TierPolicy{Tier: PlanFree, MonthlyRequestLimit: 1, AlertThreshold: 60}
	evaluation := policy.EvaluateUsage(UsageTotals{Requests: 2})
	if !evaluation.Exceeded || evaluation.Outcome != "limit_exceeded_observe_only" {
		t.Fatalf("tier overage did not remain advisory: %+v", evaluation)
	}
}

func TestBillingExportHandlerDoesNotExposeLedgerErrors(t *testing.T) {
	handler, err := NewBillingExportHandler(BillingExportHandlerOptions{
		Authorize: func(*http.Request) bool { return true },
		ResolveTenant: func(*http.Request) (string, error) {
			return "trusted-tenant", nil
		},
		Ledger: &billingTestLedger{err: errors.New("private storage detail")},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, BillingUsageExportPath+"?month=2026-11", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || strings.Contains(recorder.Body.String(), "private storage detail") {
		t.Fatalf("storage failure was exposed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
