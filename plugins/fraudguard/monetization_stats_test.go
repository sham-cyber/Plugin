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
	"time"
)

func TestMonetizationStatsRequiresAuthorizationAndStaysAggregateOnly(t *testing.T) {
	plugin, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer plugin.Cleanup()
	handler, err := NewAdminHandler(plugin, func(request *http.Request) bool {
		return request.Header.Get("X-Internal-Admin") == "authorized"
	})
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/admin/fraudguard/monetization", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized request returned %d", unauthorized.Code)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/fraudguard/monetization", nil)
	request.Header.Set("X-Internal-Admin", "authorized")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected monetization response: status=%d headers=%v", response.Code, response.Header())
	}
	var stats MonetizationStats
	if err := json.Unmarshal(response.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.SchemaVersion != "fraudguard.monetization.v1" ||
		stats.RequestHandling != "observe_only_pass_through" ||
		stats.FraudGuardBillingConnected ||
		stats.FinancialActualsAvailable ||
		stats.TierDistributionScope != "not_configured" ||
		stats.GeneratedAt.IsZero() || time.Since(stats.GeneratedAt) > time.Minute {
		t.Fatalf("monetization endpoint overstates connected financial data: %+v", stats)
	}
	for _, field := range []string{
		"tenant_id", "blocked_requests", "blocked_financial_risk_count",
		"provider_api_key", "profit", "actual_revenue",
	} {
		if containsJSONField(response.Body.String(), field) {
			t.Fatalf("monetization endpoint exposed unsupported field %q: %s", field, response.Body.String())
		}
	}
}

func TestMonetizationStatsCanUseTrustedTierAggregate(t *testing.T) {
	plugin, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer plugin.Cleanup()
	resolverCalls := 0
	handler, err := NewAdminHandlerWithOptions(plugin, AdminHandlerOptions{
		Authorize: func(request *http.Request) bool {
			return request.Header.Get("X-Internal-Admin") == "authorized"
		},
		TierDistribution: func(_ context.Context) (map[PlanTier]int64, error) {
			resolverCalls++
			return map[PlanTier]int64{PlanFree: 14, PlanPro: 3}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/admin/fraudguard/monetization", nil))
	if unauthorized.Code != http.StatusUnauthorized || resolverCalls != 0 {
		t.Fatalf("unauthorized request reached tier resolver: status=%d calls=%d", unauthorized.Code, resolverCalls)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/fraudguard/monetization", nil)
	request.Header.Set("X-Internal-Admin", "authorized")
	handler.ServeHTTP(response, request)
	var stats MonetizationStats
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &stats) != nil {
		t.Fatalf("trusted tier aggregate response failed: status=%d body=%s", response.Code, response.Body.String())
	}
	if resolverCalls != 1 || stats.TierDistributionScope != "host_authoritative_aggregate" ||
		stats.TierDistribution[PlanFree] != 14 || stats.TierDistribution[PlanPro] != 3 {
		t.Fatalf("authoritative aggregate was not reported: calls=%d stats=%+v", resolverCalls, stats)
	}
	for _, privateField := range []string{"tenant_id", "blocked_requests", "provider_api_key"} {
		if containsJSONField(response.Body.String(), privateField) {
			t.Fatalf("monetization response exposed private/unsupported field %q: %s", privateField, response.Body.String())
		}
	}
}

func TestMonetizationStatsHidesTierResolverFailures(t *testing.T) {
	plugin, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer plugin.Cleanup()
	handler, err := NewAdminHandlerWithOptions(plugin, AdminHandlerOptions{
		Authorize: func(*http.Request) bool { return true },
		TierDistribution: func(context.Context) (map[PlanTier]int64, error) {
			return nil, errors.New("sensitive billing system detail")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin/fraudguard/monetization", nil))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "sensitive billing system detail") {
		t.Fatalf("tier source failure leaked details: status=%d body=%s", response.Code, response.Body.String())
	}
}
