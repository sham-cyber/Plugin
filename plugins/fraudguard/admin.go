// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type AdminAuthorizer func(*http.Request) bool

// TierDistributionResolver must return aggregate plan counts from a trusted
// host source without tenant identifiers.
type TierDistributionResolver func(context.Context) (map[PlanTier]int64, error)

type AdminHandlerOptions struct {
	Authorize        AdminAuthorizer
	TierDistribution TierDistributionResolver
}

type AdminStatus struct {
	SchemaVersion      string               `json:"schema_version"`
	Plugin             string               `json:"plugin"`
	InferenceAvailable bool                 `json:"inference_available"`
	Build              BuildInfo            `json:"build"`
	Security           SecurityCheckReport  `json:"security"`
	Engine             Metrics              `json:"engine"`
	Telemetry          TelemetrySnapshot    `json:"telemetry"`
	Product            ProductMetrics       `json:"product"`
	Budget             BudgetMetrics        `json:"budget"`
	RedisCircuit       CircuitBreakerStatus `json:"redis_circuit"`
	ConfigReload       RuntimeConfigStatus  `json:"config_reload"`
}

// AdminDashboard is a combined process-wide snapshot for an authenticated
// internal dashboard. It intentionally contains no per-tenant or blocked-request
// totals because those are not maintained by the observe-only detector.
type AdminDashboard struct {
	GeneratedAt time.Time `json:"generated_at"`
	Status      string    `json:"status"`
	AdminStatus
}

// NewAdminHandler creates authenticated aggregate-only diagnostics. The host
// must provide authorization and bind the handler to a private admin listener.
// There is deliberately no tenant inspection, penalty-box, or block-log route.
func NewAdminHandler(plugin *Plugin, authorize AdminAuthorizer) (http.Handler, error) {
	return NewAdminHandlerWithOptions(plugin, AdminHandlerOptions{Authorize: authorize})
}

// NewAdminHandlerWithOptions creates authenticated aggregate-only diagnostics.
// A host may supply authoritative tier counts; otherwise only the current
// process's cached policy distribution is reported.
func NewAdminHandlerWithOptions(plugin *Plugin, options AdminHandlerOptions) (http.Handler, error) {
	if plugin == nil || plugin.engine == nil {
		return nil, errors.New("fraudguard: initialized plugin is required")
	}
	if options.Authorize == nil {
		return nil, errors.New("fraudguard: admin authorization callback is required")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !options.Authorize(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		switch r.URL.Path {
		case "/admin/fraudguard/metrics":
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			_, _ = w.Write([]byte(plugin.PrometheusText()))
		case "/admin/fraudguard/status":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(adminStatusSnapshot(plugin))
		case "/admin/fraudguard/dashboard":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			snapshot := adminStatusSnapshot(plugin)
			status := "ok"
			product := snapshot.Product
			if snapshot.RedisCircuit.State == CircuitOpen ||
				snapshot.ConfigReload.LastAttemptFail ||
				product.EventsDropped > 0 || product.EventsFailed > 0 ||
				product.MeteringFailed > 0 || product.AuditFailed > 0 ||
				product.WebhookFailed > 0 || product.PolicyLookupFailed > 0 ||
				product.MissingTenant > 0 {
				status = "degraded"
			}
			_ = json.NewEncoder(w).Encode(AdminDashboard{
				GeneratedAt: time.Now().UTC(),
				Status:      status,
				AdminStatus: snapshot,
			})
		case "/admin/fraudguard/monetization":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			stats := buildMonetizationStats(plugin)
			if options.TierDistribution != nil {
				distribution, err := options.TierDistribution(r.Context())
				if err == nil {
					distribution, err = validateTierDistribution(distribution)
				}
				if err != nil {
					w.WriteHeader(http.StatusServiceUnavailable)
					_ = json.NewEncoder(w).Encode(struct {
						Error string `json:"error"`
					}{Error: "monetization_snapshot_unavailable"})
					return
				}
				stats.TierDistribution = distribution
				stats.TierDistributionScope = "host_authoritative_aggregate"
			}
			_ = json.NewEncoder(w).Encode(stats)
		case "/admin/fraudguard/healthz":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			product := plugin.ProductMetrics()
			degraded := plugin.RedisCircuitStatus().State == CircuitOpen ||
				plugin.ConfigReloadStatus().LastAttemptFail ||
				product.EventsDropped > 0 || product.EventsFailed > 0 ||
				product.MeteringFailed > 0 || product.AuditFailed > 0 ||
				product.WebhookFailed > 0 || product.PolicyLookupFailed > 0 ||
				product.MissingTenant > 0
			status := "ok"
			if degraded {
				status = "degraded"
			}
			_ = json.NewEncoder(w).Encode(struct {
				Status             string    `json:"status"`
				InferenceAvailable bool      `json:"inference_available"`
				ProductDegraded    bool      `json:"product_degraded"`
				Build              BuildInfo `json:"build"`
			}{
				Status:             status,
				InferenceAvailable: true,
				ProductDegraded:    degraded,
				Build:              BuildMetadata(),
			})
		default:
			http.NotFound(w, r)
		}
	}), nil
}

func adminStatusSnapshot(plugin *Plugin) AdminStatus {
	return AdminStatus{
		SchemaVersion:      "fraudguard.admin.v1",
		Plugin:             PluginName,
		InferenceAvailable: true,
		Build:              BuildMetadata(),
		Security:           RunSecurityChecks(plugin.engine.configSnapshot()),
		Engine:             plugin.engine.Metrics(),
		Telemetry:          plugin.TelemetrySnapshot(),
		Product:            plugin.ProductMetrics(),
		Budget:             plugin.BudgetMetrics(),
		RedisCircuit:       plugin.RedisCircuitStatus(),
		ConfigReload:       plugin.ConfigReloadStatus(),
	}
}
