// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func postEvent(handler http.Handler, body []byte, apiKey string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, EventsPath, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func eventFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(strings.ReplaceAll(string(body), "__TEST_TIMESTAMP__", time.Now().UTC().Format(time.RFC3339Nano)))
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

func TestEventsRequireBearerAPIKey(t *testing.T) {
	handler := newTestHandler(t, 100)
	body := eventFixture(t, "model_completion.json")
	for _, key := range []string{"", "wrong-key"} {
		response := postEvent(handler, body, key)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	}
}

func TestEventsAcceptModelAndTavilyFixtures(t *testing.T) {
	handler := newTestHandler(t, 100)
	for _, name := range []string{"model_completion.json", "tavily_completion.json"} {
		response := postEvent(handler, eventFixture(t, name), testAPIKey)
		if response.Code != http.StatusAccepted {
			t.Fatalf("%s status = %d, body=%s", name, response.Code, response.Body.String())
		}
		var result EventResponse
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.SchemaVersion != eventSchemaVersion || !result.Accepted ||
			!result.Scored || result.Score == nil || result.Enforcement != "observe_only" {
			t.Fatalf("%s response = %+v", name, result)
		}
		if result.EventID == "" || result.AttemptID == "" {
			t.Fatalf("%s omitted event correlation identifiers: %+v", name, result)
		}
		for _, privateValue := range []string{testAPIKey, "profile-private-opaque"} {
			if bytes.Contains(response.Body.Bytes(), []byte(privateValue)) {
				t.Fatalf("%s response exposed private value %q", name, privateValue)
			}
		}
	}
}

func TestEventsAcceptStandardOllamaModelTags(t *testing.T) {
	handler := newTestHandler(t, 100)
	var input EventRequest
	if err := json.Unmarshal(eventFixture(t, "model_completion.json"), &input); err != nil {
		t.Fatal(err)
	}
	input.EventID = "ollama-tag-event"
	input.Model = "ollama/qwen3:0.6b"
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	response := postEvent(handler, body, testAPIKey)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
}

func TestEventsAcknowledgeLifecycleAndGuardrailWithoutScoring(t *testing.T) {
	handler := newTestHandler(t, 100)
	for _, body := range [][]byte{
		[]byte(`{"event_id":"start-event","attempt_id":"attempt-1","profile_ref":"profile-1","operation":"model_summary","event_type":"start","timestamp":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","status":"started","provider":"ollama","model":"qwen3-0.6b"}`),
		[]byte(`{"event_id":"guardrail-event","attempt_id":"attempt-1","profile_ref":"profile-1","operation":"model_summary","event_type":"citation_guardrail","timestamp":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","status":"passed","provider":"ollama","model":"qwen3-0.6b","validation_outcome":"passed"}`),
	} {
		response := postEvent(handler, body, testAPIKey)
		if response.Code != http.StatusAccepted {
			t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
		}
		var result EventResponse
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if !result.Accepted || result.Scored || result.Score != nil || result.Enforcement != "observe_only" {
			t.Fatalf("non-terminal event response = %+v", result)
		}
	}
}

func TestEventsRejectUnknownContentFieldsWithoutEchoingThem(t *testing.T) {
	handler := newTestHandler(t, 100)
	for _, body := range [][]byte{
		[]byte(`{"event_id":"event-1","attempt_id":"attempt-1","profile_ref":"profile-1","operation":"model_summary","event_type":"completion","timestamp":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","status":"completed","provider":"ollama","model":"qwen3-0.6b","input_tokens":1,"output_tokens":1,"token_count_source":"provider-reported","duration_ms":3,"prompt":"never accept this text"}`),
		[]byte(`{"event_id":"event-2","attempt_id":"attempt-2","profile_ref":"profile-1","operation":"tavily_search","event_type":"completion","timestamp":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","status":"completed","provider":"tavily","duration_ms":10,"result_count":2,"query":"private search words"}`),
	} {
		response := postEvent(handler, body, testAPIKey)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
		}
		for _, privateValue := range []string{"never accept this text", "private search words"} {
			if bytes.Contains(response.Body.Bytes(), []byte(privateValue)) {
				t.Fatalf("error response echoed forbidden input %q", privateValue)
			}
		}
	}
}

func TestEventsValidateContractLimits(t *testing.T) {
	handler := newTestHandler(t, 100)
	valid := eventFixture(t, "tavily_completion.json")
	cases := []struct {
		name   string
		code   int
		mutate func(*EventRequest)
	}{
		{
			name: "duration too large", code: http.StatusBadRequest,
			mutate: func(input *EventRequest) {
				value := int64(600001)
				input.DurationMS = &value
			},
		},
		{
			name: "result count too large", code: http.StatusBadRequest,
			mutate: func(input *EventRequest) {
				value := int64(1001)
				input.ResultCount = &value
			},
		},
		{
			name: "wrong status", code: http.StatusBadRequest,
			mutate: func(input *EventRequest) { input.Status = "failed" },
		},
		{name: "valid completion", code: http.StatusAccepted},
	}
	for index, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var input EventRequest
			if err := json.Unmarshal(valid, &input); err != nil {
				t.Fatal(err)
			}
			input.EventID = fmt.Sprintf("limit-event-%d", index)
			if test.mutate != nil {
				test.mutate(&input)
			}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			response := postEvent(handler, body, testAPIKey)
			if response.Code != test.code {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.code, response.Body.String())
			}
		})
	}
}

func TestEventsAreIdempotentAndRejectEventIDReuse(t *testing.T) {
	handler := newTestHandler(t, 100)
	body := eventFixture(t, "model_completion.json")
	first := postEvent(handler, body, testAPIKey)
	second := postEvent(handler, body, testAPIKey)
	if first.Code != http.StatusAccepted || second.Code != http.StatusAccepted {
		t.Fatalf("statuses = %d, %d", first.Code, second.Code)
	}
	var firstResult, secondResult EventResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstResult); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondResult); err != nil {
		t.Fatal(err)
	}
	if firstResult.Deduplicated || !secondResult.Deduplicated ||
		firstResult.Score == nil || secondResult.Score == nil ||
		firstResult.Score.RiskScore != secondResult.Score.RiskScore {
		t.Fatalf("unexpected idempotency results: first=%+v second=%+v", firstResult, secondResult)
	}

	var changed EventRequest
	if err := json.Unmarshal(body, &changed); err != nil {
		t.Fatal(err)
	}
	changed.Model = "different-model"
	changedBody, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	conflict := postEvent(handler, changedBody, testAPIKey)
	if conflict.Code != http.StatusConflict || !bytes.Contains(conflict.Body.Bytes(), []byte("event_id_conflict")) {
		t.Fatalf("reused event ID status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestConcurrentDuplicateEventsAreScoredOnce(t *testing.T) {
	handler := newTestHandler(t, 1000)
	body := eventFixture(t, "model_completion.json")
	const callers = 12
	results := make([]EventResponse, callers)
	statuses := make([]int, callers)
	var wg sync.WaitGroup
	for index := range callers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			response := postEvent(handler, body, testAPIKey)
			statuses[index] = response.Code
			if response.Code == http.StatusAccepted {
				if err := json.Unmarshal(response.Body.Bytes(), &results[index]); err != nil {
					t.Errorf("decode response: %v", err)
				}
			}
		}(index)
	}
	wg.Wait()
	nonDuplicateResponses := 0
	for index, result := range results {
		if statuses[index] != http.StatusAccepted || !result.Accepted || result.Score == nil {
			t.Fatalf("call %d status=%d result=%+v", index, statuses[index], result)
		}
		if !result.Deduplicated {
			nonDuplicateResponses++
		}
		if result.Score.RiskScore != results[0].Score.RiskScore {
			t.Fatalf("call %d risk score=%d, want %d", index, result.Score.RiskScore, results[0].Score.RiskScore)
		}
	}
	if nonDuplicateResponses != 1 {
		t.Fatalf("non-duplicate responses=%d, want exactly 1", nonDuplicateResponses)
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
