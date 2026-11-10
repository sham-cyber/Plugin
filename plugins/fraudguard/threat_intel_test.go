// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestThreatIntelOnlyAddsAdvisorySignalForTrustedInternalKey(t *testing.T) {
	feed, err := NewThreatIntelFeed(ThreatIntelOptions{MaxIndicators: 4, MaxTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	rawID := "sensitive-internal-key-id"
	if err := feed.ReplaceInternalKeyIDs([]string{rawID}, time.Minute); err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(Config{
		WarmupRequests:  1,
		AlertThreshold:  60,
		ThreatIntelFeed: feed,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	_, matched := engine.Start(Request{VirtualKeyID: rawID})
	if matched.Finding.Score != 60 || !matched.Finding.Alert ||
		!containsSignal(matched.Finding, "trusted_threat_indicator") {
		t.Fatalf("expected advisory threat signal: %+v", matched.Finding)
	}
	encoded, err := json.Marshal(matched.Finding)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), rawID) {
		t.Fatalf("raw internal ID leaked into the finding: %s", encoded)
	}

	_, unmatched := engine.Start(Request{VirtualKeyID: "different-internal-key"})
	if containsSignal(unmatched.Finding, "trusted_threat_indicator") {
		t.Fatalf("unlisted key matched the trusted feed: %+v", unmatched.Finding)
	}
}

func TestThreatIntelFeedExpiresAndAtomicallyBoundsUpdates(t *testing.T) {
	feed, err := NewThreatIntelFeed(ThreatIntelOptions{MaxIndicators: 1, MaxTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if err := feed.ReplaceInternalKeyIDs([]string{"key-a"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	beforeExpiry := time.Now().Add(30 * time.Second)
	if !feed.MatchInternalKeyID("key-a", beforeExpiry) {
		t.Fatal("unexpired trusted indicator did not match")
	}
	if err := feed.ReplaceInternalKeyIDs([]string{"key-a", "key-b"}, time.Minute); err == nil {
		t.Fatal("oversized feed update was accepted")
	}
	if feed.Size() != 1 || !feed.MatchInternalKeyID("key-a", time.Now()) {
		t.Fatal("rejected update changed the prior feed")
	}
	afterExpiry := time.Now().Add(2 * time.Minute)
	if feed.MatchInternalKeyID("key-a", afterExpiry) {
		t.Fatal("expired indicator still matched")
	}
	if removed := feed.Prune(afterExpiry); removed != 1 || feed.Size() != 0 {
		t.Fatalf("expired indicator was not pruned: removed=%d size=%d", removed, feed.Size())
	}
}

func TestThreatIntelFeedConcurrentMatchAndReplace(t *testing.T) {
	feed, err := NewThreatIntelFeed(ThreatIntelOptions{MaxIndicators: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := feed.ReplaceInternalKeyIDs([]string{"trusted-id"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 100 {
				_ = feed.MatchInternalKeyID("trusted-id", time.Now())
				_ = feed.ReplaceInternalKeyIDs([]string{"trusted-id"}, time.Hour)
			}
		}()
	}
	workers.Wait()
	if !feed.MatchInternalKeyID("trusted-id", time.Now()) {
		t.Fatal("concurrent refresh lost the trusted indicator")
	}
}
