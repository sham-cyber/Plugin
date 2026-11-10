// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
)

func TestTelemetrySnapshotsAndPrometheusOutputAreAggregateOnly(t *testing.T) {
	telemetry := NewTelemetry()
	finding := Finding{
		Score: 72, Alert: true,
		Signals: []Signal{{Name: "new_model", Reason: "model is outside baseline", Weight: 25}},
	}
	telemetry.RecordEvaluation(8*time.Millisecond, finding, true)
	telemetry.RecordCompletion()
	telemetry.RecordFinding(finding)

	snapshot := telemetry.Snapshot()
	if snapshot.Evaluations != 1 || snapshot.ProfiledRequests != 1 ||
		snapshot.CompletedRequests != 1 || snapshot.Findings != 2 || snapshot.Alerts != 2 {
		t.Fatalf("unexpected aggregate telemetry: %+v", snapshot)
	}
	if snapshot.EvaluationDuration.Count != 1 || snapshot.RiskScore.Count != 1 {
		t.Fatalf("expected duration and score histograms: %+v", snapshot)
	}
	if got := snapshot.RiskScore.Counts[7]; got != 1 {
		t.Fatalf("score 72 should be in the 70-80 bucket, got %d", got)
	}

	plugin, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer plugin.Cleanup()
	plugin.telemetry = telemetry
	text := plugin.PrometheusText()
	for _, metricName := range []string{
		"fraudguard_evaluations_total",
		"fraudguard_evaluation_duration_seconds_bucket",
		"fraudguard_risk_score_bucket",
		"fraudguard_active_profiles",
	} {
		if !strings.Contains(text, metricName) {
			t.Fatalf("Prometheus output missing %q:\n%s", metricName, text)
		}
	}
	if strings.Contains(text, "blocked") || strings.Contains(text, "tenant") || strings.Contains(text, "new_model") {
		t.Fatalf("aggregate metrics exposed an unsupported block count or sensitive labels:\n%s", text)
	}
}

func TestOpenTelemetryInstrumentsCanBeEnabledWithHostMeter(t *testing.T) {
	plugin, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer plugin.Cleanup()
	meter := noop.NewMeterProvider().Meter("fraudguard-test")
	if err := plugin.EnableOpenTelemetry(meter); err != nil {
		t.Fatalf("failed to bind host meter: %v", err)
	}
	if err := plugin.EnableOpenTelemetry(meter); err == nil {
		t.Fatal("duplicate OpenTelemetry registration should be rejected")
	}
	plugin.telemetry.RecordEvaluation(time.Millisecond, Finding{Score: 10}, true)
	plugin.telemetry.RecordCompletion()
}

func TestAdminHandlerRequiresHostAuthorizationAndExposesNoTenantRoutes(t *testing.T) {
	plugin, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer plugin.Cleanup()
	if _, err := NewAdminHandler(plugin, nil); err == nil {
		t.Fatal("admin handler must require a host authorization callback")
	}
	handler, err := NewAdminHandler(plugin, func(request *http.Request) bool {
		return request.Header.Get("X-Internal-Admin") == "authorized"
	})
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/admin/fraudguard/status", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized response status=%d", unauthorized.Code)
	}

	statusResponse := httptest.NewRecorder()
	statusRequest := httptest.NewRequest(http.MethodGet, "/admin/fraudguard/status", nil)
	statusRequest.Header.Set("X-Internal-Admin", "authorized")
	handler.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK || !strings.Contains(statusResponse.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("unexpected status response: status=%d headers=%v", statusResponse.Code, statusResponse.Header())
	}
	var status AdminStatus
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Plugin != PluginName || !status.InferenceAvailable {
		t.Fatalf("unexpected aggregate status: %+v", status)
	}
	if strings.Contains(statusResponse.Body.String(), "tenant_id") || strings.Contains(statusResponse.Body.String(), "penalty") {
		t.Fatalf("admin status exposed tenant inspection or penalty state: %s", statusResponse.Body.String())
	}

	metricsResponse := httptest.NewRecorder()
	metricsRequest := httptest.NewRequest(http.MethodGet, "/admin/fraudguard/metrics", nil)
	metricsRequest.Header.Set("X-Internal-Admin", "authorized")
	handler.ServeHTTP(metricsResponse, metricsRequest)
	if metricsResponse.Code != http.StatusOK ||
		!strings.Contains(metricsResponse.Header().Get("Content-Type"), "text/plain") ||
		!strings.Contains(metricsResponse.Body.String(), "fraudguard_evaluations_total") {
		t.Fatalf("unexpected metrics response: status=%d headers=%v body=%s",
			metricsResponse.Code, metricsResponse.Header(), metricsResponse.Body.String())
	}

	healthResponse := httptest.NewRecorder()
	healthRequest := httptest.NewRequest(http.MethodGet, "/admin/fraudguard/healthz", nil)
	healthRequest.Header.Set("X-Internal-Admin", "authorized")
	handler.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK || !strings.Contains(healthResponse.Body.String(), `"inference_available":true`) {
		t.Fatalf("unexpected health response: status=%d body=%s", healthResponse.Code, healthResponse.Body.String())
	}

	tenantRoute := httptest.NewRecorder()
	tenantRequest := httptest.NewRequest(http.MethodGet, "/admin/fraudguard/tenant", nil)
	tenantRequest.Header.Set("X-Internal-Admin", "authorized")
	handler.ServeHTTP(tenantRoute, tenantRequest)
	if tenantRoute.Code != http.StatusNotFound {
		t.Fatalf("tenant inspection route should not exist, got status=%d", tenantRoute.Code)
	}

	post := httptest.NewRecorder()
	postRequest := httptest.NewRequest(http.MethodPost, "/admin/fraudguard/metrics", nil)
	postRequest.Header.Set("X-Internal-Admin", "authorized")
	handler.ServeHTTP(post, postRequest)
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("non-GET method should be rejected, got status=%d", post.Code)
	}
}
