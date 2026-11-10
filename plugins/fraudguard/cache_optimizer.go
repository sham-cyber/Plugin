// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"container/list"
	"errors"
	"sync"
	"time"
)

const (
	defaultMaxCachedPolicies = 10_000
	maxCachedPolicies        = 100_000
)

type tierPolicyCacheEntry struct {
	key    string
	cached cachedPolicy
}

// tierPolicyLRU bounds the trusted tenant policy cache. Its keys are digests
// produced by policyCacheKey, not raw tenant identifiers.
type tierPolicyLRU struct {
	mu       sync.Mutex
	capacity int
	entries  map[string]*list.Element
	order    *list.List
}

func newTierPolicyLRU(capacity int) (*tierPolicyLRU, error) {
	if capacity < 1 || capacity > maxCachedPolicies {
		return nil, errors.New("fraudguard: policy cache capacity must be between 1 and 100000")
	}
	return &tierPolicyLRU{
		capacity: capacity,
		entries:  make(map[string]*list.Element, min(capacity, 1024)),
		order:    list.New(),
	}, nil
}

func (cache *tierPolicyLRU) get(key string, now time.Time) (cachedPolicy, bool) {
	if cache == nil || key == "" {
		return cachedPolicy{}, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()

	element, ok := cache.entries[key]
	if !ok {
		return cachedPolicy{}, false
	}
	entry := element.Value.(*tierPolicyCacheEntry)
	if !now.Before(entry.cached.expiresAt) {
		cache.remove(element)
		return cachedPolicy{}, false
	}
	cache.order.MoveToFront(element)
	return entry.cached, true
}

func (cache *tierPolicyLRU) set(key string, value cachedPolicy) {
	if cache == nil || key == "" {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if element, ok := cache.entries[key]; ok {
		element.Value.(*tierPolicyCacheEntry).cached = value
		cache.order.MoveToFront(element)
		return
	}
	element := cache.order.PushFront(&tierPolicyCacheEntry{key: key, cached: value})
	cache.entries[key] = element
	if cache.order.Len() > cache.capacity {
		cache.remove(cache.order.Back())
	}
}

func (cache *tierPolicyLRU) remove(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*tierPolicyCacheEntry)
	delete(cache.entries, entry.key)
	cache.order.Remove(element)
}

func (cache *tierPolicyLRU) len() int {
	if cache == nil {
		return 0
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return cache.order.Len()
}
