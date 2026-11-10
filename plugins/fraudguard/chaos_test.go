// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestChaosRedisDropoutDoesNotBlockOrRewriteGatewayResult(t *testing.T) {
	store, server := newTestRedisProductStore(t)
	store.timeout = 50 * time.Millisecond
	store.breaker = NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		OpenDuration:     time.Minute,
	})
	plugin, err := NewWithProductServices(Config{WarmupRequests: 1}, ProductServices{
		UsageLedger: store,
		TenantResolver: func(*schemas.BifrostContext) string {
			return "synthetic-company"
		},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	defer plugin.Cleanup()

	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyGovernanceVirtualKeyID, "synthetic-internal-id")
	request := &schemas.BifrostRequest{}
	gotRequest, shortCircuit, hookErr := plugin.PreLLMHook(ctx, request)
	if hookErr != nil || shortCircuit != nil || gotRequest != request {
		t.Fatalf("healthy request path changed before Redis failure: same=%t short=%v err=%v",
			gotRequest == request, shortCircuit, hookErr)
	}

	// Redis is optional product storage. Its outage must not change the gateway
	// result or turn an upstream timeout into a FraudGuard block.
	server.Close()
	upstreamType := "upstream_timeout"
	status := 504
	upstreamErr := &schemas.BifrostError{
		Type:           &upstreamType,
		StatusCode:     &status,
		IsBifrostError: true,
	}
	gotResponse, gotErr, hookErr := plugin.PostLLMHook(ctx, nil, upstreamErr)
	if hookErr != nil || gotResponse != nil || gotErr != upstreamErr {
		t.Fatalf("plugin changed the upstream timeout: response=%v same_error=%t hook_err=%v",
			gotResponse, gotErr == upstreamErr, hookErr)
	}

	deadline := time.Now().Add(time.Second)
	for store.CircuitStatus().State != CircuitOpen && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if status := store.CircuitStatus(); status.State != CircuitOpen {
		t.Fatalf("Redis dropout did not open the optional-service circuit: %+v", status)
	}
}

type chaosDelayedUsageLedger struct {
	started chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (l *chaosDelayedUsageLedger) RecordUsage(ctx context.Context, _ UsageEvent) error {
	l.once.Do(func() {
		select {
		case l.started <- struct{}{}:
		default:
		}
	})
	select {
	case <-l.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *chaosDelayedUsageLedger) UsageForMonth(context.Context, string, string) (UsageTotals, error) {
	return UsageTotals{}, nil
}

func TestChaosSlowProductLedgerDoesNotHoldPostHook(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	ledger := &chaosDelayedUsageLedger{
		started: make(chan struct{}, 1),
		release: release,
	}
	plugin, err := NewWithProductServices(Config{WarmupRequests: 1}, ProductServices{
		UsageLedger: ledger,
		TenantResolver: func(*schemas.BifrostContext) string {
			return "synthetic-company"
		},
		Workers:   1,
		QueueSize: 1,
		Timeout:   2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		unblock()
		_ = plugin.Cleanup()
	}()

	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyGovernanceVirtualKeyID, "synthetic-internal-id")
	request := &schemas.BifrostRequest{}
	if _, shortCircuit, hookErr := plugin.PreLLMHook(ctx, request); hookErr != nil || shortCircuit != nil {
		t.Fatalf("pre-hook unexpectedly changed request flow: short=%v err=%v", shortCircuit, hookErr)
	}
	upstreamType := "upstream_timeout"
	status := 504
	upstreamErr := &schemas.BifrostError{Type: &upstreamType, StatusCode: &status, IsBifrostError: true}
	postDone := make(chan error, 1)
	go func() {
		_, gotErr, hookErr := plugin.PostLLMHook(ctx, nil, upstreamErr)
		if hookErr == nil && gotErr != upstreamErr {
			hookErr = context.Canceled
		}
		postDone <- hookErr
	}()

	select {
	case err := <-postDone:
		if err != nil {
			t.Fatalf("post-hook changed the upstream error: %v", err)
		}
	case <-time.After(time.Second):
		unblock()
		t.Fatal("post-hook waited for the slow optional ledger")
	}
	select {
	case <-ledger.started:
	case <-time.After(time.Second):
		t.Fatal("slow ledger was not exercised")
	}
}
