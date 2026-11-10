// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRedisStoreCircuitProtectsOptionalServices(t *testing.T) {
	store, server := newTestRedisProductStore(t)
	store.breaker = NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1, OpenDuration: time.Minute})
	server.Close()

	event := UsageEvent{
		EventID:    "redis-failure-0001",
		TenantID:   "trusted-tenant",
		OccurredAt: time.Now().UTC(),
	}
	if err := store.RecordUsage(context.Background(), event); err == nil {
		t.Fatal("expected Redis failure")
	}
	if status := store.CircuitStatus(); status.State != CircuitOpen || status.LastFailureCode == "" {
		t.Fatalf("Redis outage did not open the circuit: %+v", status)
	}
	if err := store.RecordUsage(context.Background(), event); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("open circuit did not fail fast: %v", err)
	}
}

func TestRedisURLCanStartWithoutInitialPingInFailOpenMode(t *testing.T) {
	store, err := NewRedisProductStoreFromURL(context.Background(), "redis://127.0.0.1:1/0", RedisStoreOptions{
		HMACKey:           []byte("test-only-hmac-key-012345678901234567890123456789"),
		FailOpenOnStartup: true,
		OperationTimeout:  100 * time.Millisecond,
		BreakerConfig: CircuitBreakerConfig{
			FailureThreshold: 1,
			OpenDuration:     time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("fail-open setup should not require an initial Redis connection: %v", err)
	}
	defer store.Close()

	_, err = store.UsageForMonth(context.Background(), "trusted-tenant", "2026-11")
	if err == nil {
		t.Fatal("expected unavailable Redis read")
	}
	if status := store.CircuitStatus(); status.State != CircuitOpen || status.LastFailureCode == "" {
		t.Fatalf("unavailable Redis did not update circuit status: %+v", status)
	}
}
