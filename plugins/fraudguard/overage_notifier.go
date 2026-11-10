// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	defaultOverageAlertEntries = 10_000
	maxOverageAlertEntries     = 100_000
	maxOverageAlertRetention   = 400 * 24 * time.Hour
)

type OverageStage string

const (
	OverageStageNearLimit OverageStage = "near_limit"
	OverageStageExceeded  OverageStage = "exceeded"
)

type OverageNotifierConfig struct {
	MaxEntries int
	Retention  time.Duration
}

// OverageAlertDeduper claims at most one notification per tenant, month, tier,
// and threshold stage. Implementations must not retain a raw tenant identifier.
type OverageAlertDeduper interface {
	ClaimOverageAlert(context.Context, string, string, PlanTier, OverageStage) (bool, error)
	ReleaseOverageAlert(context.Context, string, string, PlanTier, OverageStage) error
}

// OverageNotifier is the bounded, process-local fallback when the host ledger
// does not provide distributed deduplication.
type OverageNotifier struct {
	mu        sync.Mutex
	key       [32]byte
	max       int
	retention time.Duration
	claimed   map[string]time.Time
}

func NewOverageNotifier(config OverageNotifierConfig) (*OverageNotifier, error) {
	if config.MaxEntries <= 0 {
		config.MaxEntries = defaultOverageAlertEntries
	}
	if config.MaxEntries > maxOverageAlertEntries {
		return nil, errors.New("fraudguard: overage notifier cannot track more than 100000 alert keys")
	}
	if config.Retention <= 0 {
		config.Retention = maxOverageAlertRetention
	}
	if config.Retention > maxOverageAlertRetention {
		return nil, errors.New("fraudguard: overage alert retention cannot exceed 400 days")
	}
	notifier := &OverageNotifier{
		max:       config.MaxEntries,
		retention: config.Retention,
		claimed:   make(map[string]time.Time),
	}
	if _, err := rand.Read(notifier.key[:]); err != nil {
		return nil, errors.New("fraudguard: unable to initialize overage alert privacy key")
	}
	return notifier, nil
}

func (n *OverageNotifier) ClaimOverageAlert(_ context.Context, tenantID, month string, tier PlanTier, stage OverageStage) (bool, error) {
	if n == nil {
		return false, errors.New("fraudguard: overage notifier is not initialized")
	}
	if err := validateOverageAlertIdentity(tenantID, month, tier, stage); err != nil {
		return false, err
	}
	now := time.Now()
	key := n.keyFor(tenantID, month, tier, stage)
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pruneLocked(now)
	if _, found := n.claimed[key]; found {
		return false, nil
	}
	if len(n.claimed) >= n.max {
		n.evictOldestLocked()
	}
	n.claimed[key] = now
	return true, nil
}

func (n *OverageNotifier) ReleaseOverageAlert(_ context.Context, tenantID, month string, tier PlanTier, stage OverageStage) error {
	if n == nil {
		return errors.New("fraudguard: overage notifier is not initialized")
	}
	if err := validateOverageAlertIdentity(tenantID, month, tier, stage); err != nil {
		return err
	}
	n.mu.Lock()
	delete(n.claimed, n.keyFor(tenantID, month, tier, stage))
	n.mu.Unlock()
	return nil
}

func (n *OverageNotifier) Prune(now time.Time) int {
	if n == nil {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.pruneLocked(now)
}

func (n *OverageNotifier) pruneLocked(now time.Time) int {
	removed := 0
	for key, claimedAt := range n.claimed {
		if now.Sub(claimedAt) >= n.retention {
			delete(n.claimed, key)
			removed++
		}
	}
	return removed
}

func (n *OverageNotifier) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	for key, claimedAt := range n.claimed {
		if oldestKey == "" || claimedAt.Before(oldest) {
			oldestKey = key
			oldest = claimedAt
		}
	}
	if oldestKey != "" {
		delete(n.claimed, oldestKey)
	}
}

func (n *OverageNotifier) keyFor(tenantID, month string, tier PlanTier, stage OverageStage) string {
	mac := hmac.New(sha256.New, n.key[:])
	_, _ = mac.Write([]byte("fraudguard:overage:v1:" + strings.TrimSpace(tenantID) + ":" + month + ":" + string(tier) + ":" + string(stage)))
	return hex.EncodeToString(mac.Sum(nil))
}

func validateOverageAlertIdentity(tenantID, month string, tier PlanTier, stage OverageStage) error {
	if strings.TrimSpace(tenantID) == "" {
		return ErrInvalidTenantID
	}
	if !validUsageMonth(month) {
		return errors.New("fraudguard: overage alert month must use YYYY-MM format")
	}
	if !tier.Valid() {
		return errors.New("fraudguard: overage alert tier is invalid")
	}
	if stage != OverageStageNearLimit && stage != OverageStageExceeded {
		return errors.New("fraudguard: overage alert stage is invalid")
	}
	return nil
}

// ClaimOverageAlert uses Redis SET NX so concurrent gateway replicas share the
// same notification claim without storing raw tenant IDs or event contents.
func (s *RedisProductStore) ClaimOverageAlert(ctx context.Context, tenantID, month string, tier PlanTier, stage OverageStage) (bool, error) {
	if s == nil || s.client == nil {
		return false, errors.New("fraudguard: Redis product store is not initialized")
	}
	if err := validateOverageAlertIdentity(tenantID, month, tier, stage); err != nil {
		return false, err
	}
	tenantRef, err := s.tenantRef(tenantID)
	if err != nil {
		return false, err
	}
	key := s.tenantKey(tenantRef, "overage-alert:"+month+":"+string(tier)+":"+string(stage))
	var claimed bool
	err = s.withRedis(ctx, func(opCtx context.Context) error {
		var setErr error
		claimed, setErr = s.client.SetNX(opCtx, key, "1", s.retention).Result()
		return setErr
	})
	if err != nil {
		return false, errors.New("fraudguard: could not claim overage notification")
	}
	return claimed, nil
}

func (s *RedisProductStore) ReleaseOverageAlert(ctx context.Context, tenantID, month string, tier PlanTier, stage OverageStage) error {
	if s == nil || s.client == nil {
		return errors.New("fraudguard: Redis product store is not initialized")
	}
	if err := validateOverageAlertIdentity(tenantID, month, tier, stage); err != nil {
		return err
	}
	tenantRef, err := s.tenantRef(tenantID)
	if err != nil {
		return err
	}
	key := s.tenantKey(tenantRef, "overage-alert:"+month+":"+string(tier)+":"+string(stage))
	if err := s.withRedis(ctx, func(opCtx context.Context) error {
		return s.client.Del(opCtx, key).Err()
	}); err != nil {
		return errors.New("fraudguard: could not release overage notification claim")
	}
	return nil
}
