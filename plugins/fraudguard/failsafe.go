// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("fraudguard: Redis circuit breaker is open")

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type CircuitBreakerConfig struct {
	FailureThreshold int
	OpenDuration     time.Duration
}

type CircuitBreakerStatus struct {
	State               CircuitState `json:"state"`
	ConsecutiveFailures int          `json:"consecutive_failures"`
	OpenUntil           time.Time    `json:"open_until,omitempty"`
	LastFailureCode     string       `json:"last_failure_code,omitempty"`
	ProbeInFlight       bool         `json:"probe_in_flight"`
}

// CircuitBreaker protects optional Redis product services. It never decides
// whether an inference or payment request may proceed.
type CircuitBreaker struct {
	mu     sync.Mutex
	config CircuitBreakerConfig
	status CircuitBreakerStatus
}

func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 3
	}
	if config.FailureThreshold > 100 {
		config.FailureThreshold = 100
	}
	if config.OpenDuration <= 0 {
		config.OpenDuration = 5 * time.Second
	}
	if config.OpenDuration > time.Hour {
		config.OpenDuration = time.Hour
	}
	return &CircuitBreaker{
		config: config,
		status: CircuitBreakerStatus{State: CircuitClosed},
	}
}

func (b *CircuitBreaker) Do(ctx context.Context, operation func(context.Context) error) error {
	if b == nil {
		return errors.New("fraudguard: circuit breaker is not initialized")
	}
	if operation == nil {
		return errors.New("fraudguard: circuit breaker operation is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	b.mu.Lock()
	now := time.Now()
	switch b.status.State {
	case CircuitOpen:
		if now.Before(b.status.OpenUntil) {
			b.mu.Unlock()
			return ErrCircuitOpen
		}
		b.status.State = CircuitHalfOpen
		b.status.ProbeInFlight = true
	case CircuitHalfOpen:
		if b.status.ProbeInFlight {
			b.mu.Unlock()
			return ErrCircuitOpen
		}
		b.status.ProbeInFlight = true
	}
	b.mu.Unlock()

	err := operation(ctx)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.ProbeInFlight = false
	if err == nil {
		b.status = CircuitBreakerStatus{State: CircuitClosed}
		return nil
	}
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return err
	}

	b.status.ConsecutiveFailures++
	b.status.LastFailureCode = classifyFailure(err)
	if b.status.State == CircuitHalfOpen || b.status.ConsecutiveFailures >= b.config.FailureThreshold {
		b.status.State = CircuitOpen
		b.status.OpenUntil = time.Now().Add(b.config.OpenDuration)
	}
	return err
}

func (b *CircuitBreaker) Status() CircuitBreakerStatus {
	if b == nil {
		return CircuitBreakerStatus{State: CircuitClosed}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status
}

func classifyFailure(err error) string {
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return "timeout"
	}
	return "unavailable"
}
