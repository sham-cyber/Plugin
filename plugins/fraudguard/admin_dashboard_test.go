// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminDashboardIsAuthenticatedAndAggregateOnly(t *testing.T) {
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
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/admin/fraudguard/dashboard", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("dashboard without authorization returned %d", unauthorized.Code)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/fraudguard/dashboard", nil)
	request.Header.Set("X-Internal-Admin", "authorized")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected dashboard response: status=%d headers=%v", response.Code, response.Header())
	}
	var dashboard AdminDashboard
	if err := json.Unmarshal(response.Body.Bytes(), &dashboard); err != nil {
		t.Fatal(err)
	}
	if dashboard.Status != "ok" || !dashboard.InferenceAvailable ||
		dashboard.SchemaVersion != "fraudguard.admin.v1" || dashboard.GeneratedAt.IsZero() {
		t.Fatalf("unexpected dashboard snapshot: %+v", dashboard)
	}
	if dashboard.Build.Version == "" || dashboard.Build.Commit == "" ||
		dashboard.Security.Status != "configuration_valid" {
		t.Fatalf("dashboard is missing release/security status: %+v", dashboard)
	}
	if time.Since(dashboard.GeneratedAt) > time.Minute {
		t.Fatalf("dashboard timestamp is not recent: %s", dashboard.GeneratedAt)
	}
	for _, privateField := range []string{"tenant_id", "active_tenants", "blocked_requests", "blocked_injections"} {
		if containsJSONField(response.Body.String(), privateField) {
			t.Fatalf("dashboard included unsupported field %q", privateField)
		}
	}
}

func containsJSONField(body, field string) bool {
	var values map[string]json.RawMessage
	if json.Unmarshal([]byte(body), &values) != nil {
		return false
	}
	if _, found := values[field]; found {
		return true
	}
	for _, value := range values {
		var nested map[string]json.RawMessage
		if json.Unmarshal(value, &nested) == nil {
			if _, found := nested[field]; found {
				return true
			}
		}
	}
	return false
}
