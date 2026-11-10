// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"time"
)

const (
	defaultHealthRedisTimeout = 500 * time.Millisecond
	maxHealthRedisTimeout     = 5 * time.Second
)

// RedisHealthChecker is implemented by host product stores that can safely
// answer a bounded operator health probe.
type RedisHealthChecker interface {
	HealthCheck(context.Context) error
}

// HealthOptions controls diagnostic probes. Memory is process-wide, not an
// estimate of memory attributable only to FraudGuard.
type HealthOptions struct {
	RedisTimeout             time.Duration
	MaxHeapAllocBytes        uint64
	RequireRedisForReadiness bool
}

// HealthStatus contains aggregate process/plugin health only. Reasons are
// stable codes; provider errors, Redis URLs, tenant IDs, keys, and prompts are
// never included.
type HealthStatus struct {
	SchemaVersion      string               `json:"schema_version"`
	Status             string               `json:"status"`
	Ready              bool                 `json:"ready"`
	InferenceAvailable bool                 `json:"inference_available"`
	Build              BuildInfo            `json:"build"`
	Security           *SecurityCheckReport `json:"security,omitempty"`
	RedisStatus        string               `json:"redis_status"`
	HeapAllocBytes     uint64               `json:"heap_alloc_bytes"`
	MaxHeapAllocBytes  uint64               `json:"max_heap_alloc_bytes,omitempty"`
	Reasons            []string             `json:"reasons,omitempty"`
	CheckedAt          time.Time            `json:"checked_at"`
}

// CheckHealth performs operator-only checks outside the inference path.
// Redis is optional by default; its outage marks product services degraded but
// does not make observe-only inference unavailable.
func (p *Plugin) CheckHealth(ctx context.Context, options HealthOptions) HealthStatus {
	status := HealthStatus{
		SchemaVersion:     "fraudguard.health.v1",
		Status:            "unavailable",
		Build:             BuildMetadata(),
		RedisStatus:       "not_configured",
		MaxHeapAllocBytes: options.MaxHeapAllocBytes,
		CheckedAt:         time.Now().UTC(),
	}
	if p == nil || p.engine == nil {
		status.Reasons = []string{"plugin_uninitialized"}
		return status
	}

	status.InferenceAvailable = true
	status.Ready = true
	securityReport := RunSecurityChecks(p.engine.configSnapshot())
	status.Security = &securityReport
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	status.HeapAllocBytes = memStats.HeapAlloc
	if options.MaxHeapAllocBytes > 0 && status.HeapAllocBytes > options.MaxHeapAllocBytes {
		status.Ready = false
		status.Reasons = append(status.Reasons, "process_heap_threshold_exceeded")
	}

	checker, configured := p.redisHealthChecker()
	if configured && checker == nil {
		status.RedisStatus = "unsupported"
		status.Reasons = append(status.Reasons, "redis_health_check_unsupported")
		if options.RequireRedisForReadiness {
			status.Ready = false
		}
	} else if checker != nil {
		timeout := options.RedisTimeout
		if timeout <= 0 {
			timeout = defaultHealthRedisTimeout
		}
		if timeout > maxHealthRedisTimeout {
			timeout = maxHealthRedisTimeout
		}
		if ctx == nil {
			ctx = context.Background()
		}
		redisCtx, cancel := context.WithTimeout(ctx, timeout)
		err := checker.HealthCheck(redisCtx)
		cancel()
		if err != nil {
			status.RedisStatus = "unavailable"
			status.Reasons = append(status.Reasons, "redis_unavailable")
			if options.RequireRedisForReadiness {
				status.Ready = false
			}
		} else {
			status.RedisStatus = "ok"
		}
	} else if options.RequireRedisForReadiness {
		status.Ready = false
		status.Reasons = append(status.Reasons, "redis_required_not_configured")
	}

	status.Status = "ok"
	if !status.Ready || len(status.Reasons) > 0 {
		status.Status = "degraded"
	}
	return status
}

func (p *Plugin) redisHealthChecker() (RedisHealthChecker, bool) {
	if p == nil || p.product == nil {
		return nil, false
	}
	services := p.product.services
	configured := services.UsageLedger != nil || services.AuditLog != nil || services.PlanResolver != nil
	for _, service := range []any{services.UsageLedger, services.AuditLog, services.PlanResolver} {
		if checker, ok := service.(RedisHealthChecker); ok {
			return checker, true
		}
	}
	return nil, configured
}

// NewProbeHandler provides authenticated /livez, /healthz, and /readyz routes.
// Hosts must bind it to a private listener and supply auth supported by their
// probe environment; it is not automatically mounted into Bifrost.
func NewProbeHandler(plugin *Plugin, authorize AdminAuthorizer, options HealthOptions) (http.Handler, error) {
	if plugin == nil || plugin.engine == nil {
		return nil, errors.New("fraudguard: initialized plugin is required")
	}
	if authorize == nil {
		return nil, errors.New("fraudguard: probe authorization callback is required")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !authorize(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.URL.Path {
		case "/livez":
			_ = json.NewEncoder(w).Encode(struct {
				Status             string    `json:"status"`
				InferenceAvailable bool      `json:"inference_available"`
				Build              BuildInfo `json:"build"`
			}{Status: "ok", InferenceAvailable: true, Build: BuildMetadata()})
		case "/healthz", "/readyz":
			status := plugin.CheckHealth(r.Context(), options)
			if r.URL.Path == "/readyz" && !status.Ready {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			_ = json.NewEncoder(w).Encode(status)
		default:
			http.NotFound(w, r)
		}
	}), nil
}
