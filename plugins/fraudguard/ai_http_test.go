// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBifrostHTTPAIAnalyzerSendsOnlySanitizedFinding(t *testing.T) {
	const hostKey = "private-host-virtual-key"
	var (
		mu      sync.Mutex
		gotBody string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+hostKey {
			t.Errorf("Authorization header was not set to the host virtual key")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["model"] != "fraud-review-model" {
			t.Errorf("unexpected model: %v", body["model"])
		}
		messages, _ := body["messages"].([]any)
		if len(messages) != 2 {
			t.Errorf("got %d messages, want system and sanitized summary", len(messages))
		}
		mu.Lock()
		raw, _ := json.Marshal(body)
		gotBody = string(raw)
		mu.Unlock()

		content := `{"conclusion":"review_suggested","confidence":74,"explanation":"The combined behavior signals deserve operator review."}`
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": content}}},
		})
	}))
	defer server.Close()

	analyzer, err := NewBifrostHTTPAIAnalyzer(BifrostHTTPAIAnalyzerConfig{
		Endpoint: server.URL + "/v1/chat/completions",
		Model:    "fraud-review-model",
		APIKey:   hostKey,
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := analyzer.Analyze(context.Background(), AIAnalysisInput{
		Score: 68, Threshold: 60, Alert: true,
		Signals: []Signal{{
			Name:   "rapid_sequence",
			Reason: "PRIVATE_REASON_MUST_NOT_BE_SENT",
			Weight: 35,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Conclusion != AIConclusionReviewSuggested || result.Confidence != 74 {
		t.Fatalf("unexpected AI advisory: %+v", result)
	}

	mu.Lock()
	body := gotBody
	mu.Unlock()
	if body == "" {
		t.Fatal("mock Bifrost endpoint did not receive a request")
	}
	for _, forbidden := range []string{hostKey, "PRIVATE_REASON_MUST_NOT_BE_SENT", "prompt", "virtual_key"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(forbidden)) {
			t.Fatalf("AI request contains forbidden value %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, "rapid_sequence") {
		t.Fatalf("AI request omitted the fixed signal name: %s", body)
	}
}

func TestBifrostHTTPAIAnalyzerRequiresSecureEndpoint(t *testing.T) {
	for _, endpoint := range []string{
		"http://model.example.com/v1/chat/completions",
		"https://model.example.com/v1/chat/completions?token=secret",
		"https://user:secret@model.example.com/v1/chat/completions",
	} {
		if _, err := NewBifrostHTTPAIAnalyzer(BifrostHTTPAIAnalyzerConfig{
			Endpoint: endpoint, Model: "review", APIKey: "secret",
		}); err == nil {
			t.Errorf("accepted unsafe endpoint %q", endpoint)
		}
	}
	if _, err := NewBifrostHTTPAIAnalyzer(BifrostHTTPAIAnalyzerConfig{
		Endpoint: "http://127.0.0.1:8080/v1/chat/completions",
		Model:    "review",
		APIKey:   "secret",
	}); err != nil {
		t.Fatalf("rejected loopback Bifrost endpoint: %v", err)
	}
}

func TestBifrostHTTPAIAnalyzerRejectsUnknownSignalNames(t *testing.T) {
	analyzer, err := NewBifrostHTTPAIAnalyzer(BifrostHTTPAIAnalyzerConfig{
		Endpoint: "http://127.0.0.1:8080/v1/chat/completions",
		Model:    "review",
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = analyzer.Analyze(context.Background(), AIAnalysisInput{
		Score: 80, Threshold: 60, Alert: true,
		Signals: []Signal{{Name: "private-customer-value", Weight: 50}},
	})
	if err == nil {
		t.Fatal("expected an unknown signal name to be rejected before sending")
	}
}

func TestBifrostHTTPAIAnalyzerDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/v1/chat/completions", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	analyzer, err := NewBifrostHTTPAIAnalyzer(BifrostHTTPAIAnalyzerConfig{
		Endpoint: redirect.URL + "/v1/chat/completions",
		Model:    "review",
		APIKey:   "private-key",
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = analyzer.Analyze(context.Background(), AIAnalysisInput{
		Score: 80, Threshold: 60, Alert: true,
		Signals: []Signal{{Name: "rapid_sequence", Weight: 30}},
	})
	if err == nil {
		t.Fatal("expected redirect response to be rejected")
	}
	if redirected.Load() {
		t.Fatal("AI review request followed a redirect to another host")
	}
}

func TestBifrostHTTPAIAnalyzerFromEnvironmentRequiresDedicatedKeyIdentity(t *testing.T) {
	t.Setenv(envAIReviewEndpoint, "")
	t.Setenv(envAIReviewModel, "")
	t.Setenv(envAIReviewAPIKey, "")
	t.Setenv(envAIReviewKeyID, "")
	if _, _, err := NewBifrostHTTPAIAnalyzerFromEnvironment(time.Second); err == nil {
		t.Fatal("expected incomplete AI review environment to fail")
	} else if strings.Contains(err.Error(), "private") {
		t.Fatalf("error exposed a secret: %v", err)
	}
}
