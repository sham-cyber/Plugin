// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestCircuitBreakerOpensAndAllowsOneRecoveryProbe(t *testing.T) {
	breaker := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		OpenDuration:     20 * time.Millisecond,
	})
	fail := func(context.Context) error { return errors.New("private Redis endpoint error") }
	for range 2 {
		if err := breaker.Do(context.Background(), fail); err == nil {
			t.Fatal("expected operation failure")
		}
	}
	status := breaker.Status()
	if status.State != CircuitOpen || status.ConsecutiveFailures != 2 || status.LastFailureCode != "unavailable" {
		t.Fatalf("unexpected open state: %+v", status)
	}
	if err := breaker.Do(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("open circuit should reject the Redis operation: %v", err)
	}

	time.Sleep(25 * time.Millisecond)
	entered := make(chan struct{})
	release := make(chan struct{})
	probeResult := make(chan error, 1)
	go func() {
		probeResult <- breaker.Do(context.Background(), func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	if err := breaker.Do(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("half-open state should permit only one probe: %v", err)
	}
	close(release)
	if err := <-probeResult; err != nil {
		t.Fatalf("recovery probe failed: %v", err)
	}
	if status := breaker.Status(); status.State != CircuitClosed || status.ConsecutiveFailures != 0 {
		t.Fatalf("successful probe did not close circuit: %+v", status)
	}
}

func TestCircuitBreakerClassifiesTimeoutWithoutLeakingError(t *testing.T) {
	breaker := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1})
	err := breaker.Do(context.Background(), func(context.Context) error {
		return &net.DNSError{IsTimeout: true, Err: "sensitive-hostname"}
	})
	if err == nil {
		t.Fatal("expected timeout")
	}
	status := breaker.Status()
	if status.State != CircuitOpen || status.LastFailureCode != "timeout" {
		t.Fatalf("timeout was not classified safely: %+v", status)
	}
	if status.LastFailureCode == err.Error() {
		t.Fatal("raw error text was exposed in breaker status")
	}
}
