// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	defaultThreatIndicatorLimit = 10_000
	maxThreatIndicatorLimit     = 100_000
	maxThreatIndicatorTTL       = 30 * 24 * time.Hour
)

type ThreatIntelOptions struct {
	MaxIndicators int
	MaxTTL        time.Duration
}

// ThreatIntelFeed holds only HMAC digests of trusted internal virtual-key IDs.
// It never reads headers, IP addresses, user agents, or caller fingerprints.
type ThreatIntelFeed struct {
	mu         sync.RWMutex
	key        [32]byte
	maxEntries int
	maxTTL     time.Duration
	indicators map[string]time.Time
}

func NewThreatIntelFeed(options ThreatIntelOptions) (*ThreatIntelFeed, error) {
	if options.MaxIndicators <= 0 {
		options.MaxIndicators = defaultThreatIndicatorLimit
	}
	if options.MaxIndicators > maxThreatIndicatorLimit {
		return nil, errors.New("fraudguard: threat indicator limit cannot exceed 100000")
	}
	if options.MaxTTL <= 0 {
		options.MaxTTL = 24 * time.Hour
	}
	if options.MaxTTL > maxThreatIndicatorTTL {
		return nil, errors.New("fraudguard: threat indicator TTL cannot exceed 30 days")
	}
	feed := &ThreatIntelFeed{
		maxEntries: options.MaxIndicators,
		maxTTL:     options.MaxTTL,
		indicators: make(map[string]time.Time),
	}
	if _, err := rand.Read(feed.key[:]); err != nil {
		return nil, errors.New("fraudguard: unable to initialize private threat-indicator hashing")
	}
	return feed, nil
}

// ReplaceInternalKeyIDs atomically replaces the advisory feed. IDs must be
// resolved by trusted host code; they are HMACed immediately and never retained.
func (f *ThreatIntelFeed) ReplaceInternalKeyIDs(ids []string, ttl time.Duration) error {
	if f == nil {
		return errors.New("fraudguard: threat indicator feed is not initialized")
	}
	if ttl <= 0 {
		ttl = f.maxTTL
	}
	if ttl > f.maxTTL {
		return errors.New("fraudguard: threat indicator TTL exceeds the configured maximum")
	}
	if len(ids) > f.maxEntries {
		return errors.New("fraudguard: threat indicator feed exceeds its configured size limit")
	}
	expiresAt := time.Now().Add(ttl)
	next := make(map[string]time.Time, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || len(id) > 256 {
			return errors.New("fraudguard: threat indicators must be valid internal virtual-key IDs")
		}
		next[f.digest(id)] = expiresAt
	}
	f.mu.Lock()
	f.indicators = next
	f.mu.Unlock()
	return nil
}

func (f *ThreatIntelFeed) MatchInternalKeyID(id string, now time.Time) bool {
	if f == nil || strings.TrimSpace(id) == "" {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	digest := f.digest(id)
	f.mu.RLock()
	expiresAt, found := f.indicators[digest]
	f.mu.RUnlock()
	return found && now.Before(expiresAt)
}

func (f *ThreatIntelFeed) Prune(now time.Time) int {
	if f == nil {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	removed := 0
	for digest, expiresAt := range f.indicators {
		if !now.Before(expiresAt) {
			delete(f.indicators, digest)
			removed++
		}
	}
	return removed
}

func (f *ThreatIntelFeed) Size() int {
	if f == nil {
		return 0
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.indicators)
}

func (f *ThreatIntelFeed) digest(id string) string {
	mac := hmac.New(sha256.New, f.key[:])
	_, _ = mac.Write([]byte("fraudguard:trusted-threat-indicator:v1:" + id))
	return string(mac.Sum(nil))
}
