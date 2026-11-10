// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestTierPolicyLRUEvictsLeastRecentlyUsedAndExpiresEntries(t *testing.T) {
	cache, err := newTierPolicyLRU(2)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.November, 10, 12, 0, 0, 0, time.UTC)
	cache.set("tenant-digest-a", cachedPolicy{
		policy: TierPolicy{Tier: PlanFree}, expiresAt: now.Add(time.Minute),
	})
	cache.set("tenant-digest-b", cachedPolicy{
		policy: TierPolicy{Tier: PlanPro}, expiresAt: now.Add(time.Minute),
	})
	if got, ok := cache.get("tenant-digest-a", now); !ok || got.policy.Tier != PlanFree {
		t.Fatalf("expected first policy in cache: got=%+v ok=%t", got, ok)
	}

	cache.set("tenant-digest-c", cachedPolicy{
		policy: TierPolicy{Tier: PlanEnterprise}, expiresAt: now.Add(time.Minute),
	})
	if _, ok := cache.get("tenant-digest-b", now); ok {
		t.Fatal("least-recently-used policy was not evicted")
	}
	if cache.len() != 2 {
		t.Fatalf("cache size=%d, want capacity 2", cache.len())
	}

	cache.set("expired", cachedPolicy{
		policy: TierPolicy{Tier: PlanFree}, expiresAt: now,
	})
	if _, ok := cache.get("expired", now); ok {
		t.Fatal("expired policy was returned")
	}
	if cache.len() != 1 {
		t.Fatalf("expired entry remained in cache: size=%d", cache.len())
	}
}

func TestTierPolicyLRUConcurrentAccessIsSafeAndBounded(t *testing.T) {
	cache, err := newTierPolicyLRU(32)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	var workers sync.WaitGroup
	for worker := range 12 {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for i := range 500 {
				key := fmt.Sprintf("tenant-digest-%d", (worker*500+i)%64)
				cache.set(key, cachedPolicy{
					policy: TierPolicy{Tier: PlanPro}, expiresAt: now.Add(time.Hour),
				})
				cache.get(key, now)
			}
		}(worker)
	}
	workers.Wait()
	if got := cache.len(); got > 32 {
		t.Fatalf("cache grew past capacity: %d", got)
	}
}

func TestTierPolicyLRURejectsInvalidCapacity(t *testing.T) {
	for _, capacity := range []int{0, -1, maxCachedPolicies + 1} {
		if _, err := newTierPolicyLRU(capacity); err == nil {
			t.Fatalf("invalid cache capacity %d was accepted", capacity)
		}
	}
}
