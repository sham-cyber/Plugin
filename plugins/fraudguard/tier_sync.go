// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"errors"
	"strings"
	"time"
)

// TierSyncManager refreshes a cached policy after a trusted host has already
// verified and applied a subscription change to its plan resolver.
type TierSyncManager struct {
	runtime *ProductRuntime
}

func NewTierSyncManager(runtime *ProductRuntime) (*TierSyncManager, error) {
	if runtime == nil || runtime.services.PlanResolver == nil || runtime.services.Policies == nil {
		return nil, errors.New("fraudguard: tier sync requires a configured product runtime")
	}
	return &TierSyncManager{runtime: runtime}, nil
}

// RefreshAfterTrustedUpdate bypasses the policy cache. The caller is responsible
// for authenticating any billing webhook and updating the trusted plan source
// before calling this method.
func (m *TierSyncManager) RefreshAfterTrustedUpdate(ctx context.Context, tenantID string) (TierPolicy, error) {
	if m == nil || m.runtime == nil {
		return TierPolicy{}, errors.New("fraudguard: tier sync manager is not initialized")
	}
	if strings.TrimSpace(tenantID) == "" {
		return TierPolicy{}, ErrInvalidTenantID
	}
	policy, found, err := m.runtime.RefreshPolicy(ctx, tenantID)
	if err != nil {
		return TierPolicy{}, err
	}
	if !found {
		return TierPolicy{}, errors.New("fraudguard: trusted plan source is unavailable")
	}
	return policy, nil
}

// CachedTierDistribution returns counts from this process's unexpired policy
// cache only. It is not an authoritative customer or subscription census.
func (r *ProductRuntime) CachedTierDistribution() map[PlanTier]int64 {
	if r == nil {
		return map[PlanTier]int64{}
	}
	return r.policyCache.countByTier(time.Now())
}
