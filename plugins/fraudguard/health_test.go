// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/redis/go-redis/v9"
)

type testHealthStore struct {
	err error
}

func (s testHealthStore) RecordUsage(context.Context, UsageEvent) error { return nil }
func (s testHealthStore) UsageForMonth(context.Context, string, string) (UsageTotals, error) {
	return UsageTotals{}, nil
}
func (s testHealthStore) HealthCheck(context.Context) error { return s.err }

func newPluginWithHealthStore(t *testing.T, store UsageLedger) *Plugin {
	t.Helper()
	plugin, err := NewWithProductServices(Config{}, ProductServices{
		TenantResolver: func(_ *schemas.BifrostContext) string { return "synthetic-tenant" },
		UsageLedger:    store,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugin.Cleanup() })
	return plugin
}

func TestCheckHealthKeepsInferenceReadyDuringOptionalRedisFailure(t *testing.T) {
	const privateError = "redis password must not appear"
	plugin := newPluginWithHealthStore(t, testHealthStore{err: errors.New(privateError)})

	status := plugin.CheckHealth(context.Background(), HealthOptions{})
	if !status.InferenceAvailable || !status.Ready {
		t.Fatalf("optional Redis failure must not make inference unready: %+v", status)
	}
	if status.Status != "degraded" || status.RedisStatus != "unavailable" {
		t.Fatalf("expected an observable degraded product status: %+v", status)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), privateError) {
		t.Fatal("health status exposed a backend error")
	}
}

func TestCheckHealthRequiredRedisAndHeapThresholdAffectReadiness(t *testing.T) {
	plugin := newPluginWithHealthStore(t, testHealthStore{err: errors.New("unavailable")})
	status := plugin.CheckHealth(context.Background(), HealthOptions{
		RequireRedisForReadiness: true,
		MaxHeapAllocBytes:        1,
	})
	if status.Ready || status.Status != "degraded" || len(status.Reasons) != 2 {
		t.Fatalf("required Redis and heap threshold should mark readiness degraded: %+v", status)
	}
	if status.HeapAllocBytes <= status.MaxHeapAllocBytes {
		t.Fatalf("expected process heap threshold to be exceeded: %+v", status)
	}
}

func TestCheckHealthWithoutRedisIsReadyByDefault(t *testing.T) {
	plugin, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer plugin.Cleanup()
	status := plugin.CheckHealth(context.Background(), HealthOptions{})
	if !status.Ready || !status.InferenceAvailable || status.RedisStatus != "not_configured" {
		t.Fatalf("Redis is optional for inference readiness: %+v", status)
	}
}

func TestRedisProductStoreHealthCheckPingsConfiguredRedis(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	store, err := NewRedisProductStore(RedisStoreOptions{
		Client:    client,
		HMACKey:   bytes.Repeat([]byte("h"), 32),
		Namespace: "health-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := store.HealthCheck(ctx); err != nil {
		t.Fatalf("healthy local Redis ping failed: %v", err)
	}
}

func TestProbeHandlerRequiresAuthorizationAndExposesReadiness(t *testing.T) {
	plugin := newPluginWithHealthStore(t, testHealthStore{err: errors.New("unavailable")})
	authorize := func(r *http.Request) bool {
		return r.Header.Get("X-Health-Auth") == "synthetic"
	}
	handler, err := NewProbeHandler(plugin, authorize, HealthOptions{RequireRedisForReadiness: true})
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized probe status = %d", unauthorized.Code)
	}

	live := httptest.NewRequest(http.MethodGet, "/livez", nil)
	live.Header.Set("X-Health-Auth", "synthetic")
	liveResponse := httptest.NewRecorder()
	handler.ServeHTTP(liveResponse, live)
	if liveResponse.Code != http.StatusOK {
		t.Fatalf("live probe status = %d", liveResponse.Code)
	}

	ready := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	ready.Header.Set("X-Health-Auth", "synthetic")
	readyResponse := httptest.NewRecorder()
	handler.ServeHTTP(readyResponse, ready)
	if readyResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("required-Redis readiness status = %d", readyResponse.Code)
	}
	if strings.Contains(readyResponse.Body.String(), "unavailable") == false {
		t.Fatalf("readiness response omitted stable Redis status: %s", readyResponse.Body.String())
	}
}
