// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	fraudguard "github.com/maximhq/bifrost/plugins/fraudguard"
)

const testAPIKey = "fg_test_key_for_http_api"

func newTestHandler(t *testing.T, rateLimit int) http.Handler {
	t.Helper()
	digest := sha256.Sum256([]byte(testAPIKey))
	engine, err := fraudguard.NewEngine(fraudguard.Config{
		WarmupRequests: 2,
		AlertThreshold: 20,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	handler, err := NewHandler(engine, Config{
		KeyHashes:         map[string]string{hex.EncodeToString(digest[:]): "tenant-test"},
		RequestsPerMinute: rateLimit,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return handler
}

func scoreBody(t *testing.T, eventID, provider, model string, at time.Time) []byte {
	t.Helper()
	body, err := json.Marshal(ScoreRequest{
		EventID:      eventID,
		ProfileRef:   "profile-opaque-1",
		Provider:     provider,
		Model:        model,
		Timestamp:    at,
		InputTokens:  100,
		OutputTokens: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func postScore(handler http.Handler, body []byte, apiKey string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, ScorePath, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestScoreRequiresBearerAPIKey(t *testing.T) {
	handler := newTestHandler(t, 100)
	body := scoreBody(t, "event-1", "openai", "model-a", time.Now().UTC())
	for _, key := range []string{"", "wrong-key"} {
		response := postScore(handler, body, key)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	}
}

func TestScoreReturnsWarmupThenBehaviorFinding(t *testing.T) {
	handler := newTestHandler(t, 100)
	base := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Second)
	for i := 0; i < 2; i++ {
		response := postScore(handler, scoreBody(t, "base-"+string(rune('a'+i)), "openai", "model-a", base.Add(time.Duration(i)*30*time.Second)), testAPIKey)
		if response.Code != http.StatusOK {
			t.Fatalf("baseline status = %d, body=%s", response.Code, response.Body.String())
		}
		var result ScoreResponse
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if !result.Warmup || result.RiskScore != 0 {
			t.Fatalf("baseline should be quiet during warmup: %+v", result)
		}
	}

	response := postScore(handler, scoreBody(t, "event-novel", "anthropic", "model-b", base.Add(90*time.Second)), testAPIKey)
	if response.Code != http.StatusOK {
		t.Fatalf("novel event status = %d, body=%s", response.Code, response.Body.String())
	}
	var result ScoreResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Warmup || result.RiskScore == 0 || !result.Alert || result.Enforcement != "observe_only" {
		t.Fatalf("unexpected scored result: %+v", result)
	}
	if len(result.Signals) != 2 || result.Signals[0].Name != "new_model" || result.Signals[1].Name != "new_provider" {
		t.Fatalf("unexpected signals: %+v", result.Signals)
	}
}

func TestScoreRejectsUnknownPromptFields(t *testing.T) {
	handler := newTestHandler(t, 100)
	body := []byte(`{"event_id":"event-1","profile_ref":"profile-1","provider":"openai","model":"m","timestamp":"2026-11-15T10:00:00Z","input_tokens":1,"output_tokens":1,"prompt":"must not be accepted"}`)
	response := postScore(handler, body, testAPIKey)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("must not be accepted")) {
		t.Fatal("error response echoed prompt text")
	}
}

func TestScoreIsIdempotentForRepeatedEvent(t *testing.T) {
	handler := newTestHandler(t, 100)
	base := time.Now().UTC().Add(-time.Minute)
	for i := 0; i < 2; i++ {
		response := postScore(handler, scoreBody(t, "base-"+string(rune('a'+i)), "openai", "model-a", base.Add(time.Duration(i)*30*time.Second)), testAPIKey)
		if response.Code != http.StatusOK {
			t.Fatalf("baseline status = %d", response.Code)
		}
	}
	body := scoreBody(t, "same-event", "openai", "new-model", base.Add(70*time.Second))
	first := postScore(handler, body, testAPIKey)
	second := postScore(handler, body, testAPIKey)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("repeat statuses = %d, %d", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("idempotent responses differ: %q != %q", first.Body.String(), second.Body.String())
	}
	for _, privateValue := range []string{testAPIKey, "profile-opaque-1", "tenant-test"} {
		if bytes.Contains(first.Body.Bytes(), []byte(privateValue)) {
			t.Fatalf("response exposed private value %q", privateValue)
		}
	}
}

func TestScoreRateLimitIsPerTenant(t *testing.T) {
	handler := newTestHandler(t, 1)
	now := time.Now().UTC()
	first := postScore(handler, scoreBody(t, "first", "openai", "model-a", now), testAPIKey)
	second := postScore(handler, scoreBody(t, "second", "openai", "model-a", now.Add(time.Second)), testAPIKey)
	if first.Code != http.StatusOK || second.Code != http.StatusTooManyRequests {
		t.Fatalf("statuses = %d, %d; want 200, 429", first.Code, second.Code)
	}
}

func TestConcurrentDuplicateEventsReturnSameFinding(t *testing.T) {
	handler := newTestHandler(t, 1000)
	body := scoreBody(t, "concurrent-event", "openai", "model-a", time.Now().UTC())
	const callers = 12
	responses := make([]string, callers)
	statuses := make([]int, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			response := postScore(handler, body, testAPIKey)
			statuses[index] = response.Code
			responses[index] = response.Body.String()
		}(i)
	}
	wg.Wait()
	for i := 0; i < callers; i++ {
		if statuses[i] != http.StatusOK || responses[i] != responses[0] {
			t.Fatalf("call %d got status %d and response %q; expected identical 200 response", i, statuses[i], responses[i])
		}
	}
}

func TestParseKeyHashMappings(t *testing.T) {
	hash := sha256.Sum256([]byte(testAPIKey))
	encoded := hex.EncodeToString(hash[:])
	mappings, err := ParseKeyHashMappings(encoded + "=tenant-a")
	if err != nil || mappings[encoded] != "tenant-a" {
		t.Fatalf("ParseKeyHashMappings returned %v, %v", mappings, err)
	}
	if _, err := ParseKeyHashMappings(encoded + "=tenant-a;" + encoded + "=tenant-b"); err == nil {
		t.Fatal("expected duplicate API key hash to be rejected")
	}
}

func TestIdempotencyWaitHonorsContextCancellation(t *testing.T) {
	store := newIdempotencyStore(1, time.Minute)
	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = store.Do(context.Background(), "same-key", func() (ScoreResponse, error) {
			close(started)
			<-release
			return ScoreResponse{EventID: "event"}, nil
		})
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Do(ctx, "same-key", func() (ScoreResponse, error) {
		return ScoreResponse{}, nil
	}); err == nil {
		t.Fatal("a cancelled waiter should return an error")
	}
	close(release)
	<-firstDone
}
