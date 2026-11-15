// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	fraudguard "github.com/maximhq/bifrost/plugins/fraudguard"
)

func TestDecodeConfigAcceptsBifrostSnakeCase(t *testing.T) {
	config, err := decodeConfig(map[string]any{
		"alert_threshold":                float64(73),
		"warmup_requests":                float64(4),
		"enable_prompt_override_signals": true,
		"window":                         "15m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.AlertThreshold != 73 || config.WarmupRequests != 4 ||
		!config.EnablePromptOverrideSignals || config.Window != 15*time.Minute {
		t.Fatalf("config did not decode expected values: %+v", config)
	}
}

func TestDecodeConfigRejectsUnknownAndRuntimeOnlyFields(t *testing.T) {
	for _, raw := range []map[string]any{
		{"risk_threshold": float64(60)},
		{"on_finding": "not-a-callback"},
		{"ai_analyzer": "not-a-host-adapter"},
		{"version": float64(2)},
	} {
		if _, err := decodeConfig(raw); err == nil {
			t.Fatalf("decodeConfig(%v) unexpectedly succeeded", raw)
		}
	}
}

func TestSharedPluginInitializesAndPassesThrough(t *testing.T) {
	_ = Cleanup()
	t.Cleanup(func() { _ = Cleanup() })

	if err := Init(map[string]any{"alert_threshold": float64(60), "warmup_requests": float64(1)}); err != nil {
		t.Fatal(err)
	}
	if got := GetName(); got != fraudguard.PluginName {
		t.Fatalf("GetName() = %q, want %q", got, fraudguard.PluginName)
	}

	request := &schemas.BifrostRequest{}
	got, shortCircuit, err := PreLLMHook(nil, request)
	if err != nil {
		t.Fatal(err)
	}
	if got != request || shortCircuit != nil {
		t.Fatal("PreLLMHook changed or short-circuited the request")
	}
	if err := PreRequestHook(nil, request); err != nil {
		t.Fatal(err)
	}
	response := &schemas.BifrostResponse{}
	gotResponse, gotErr, err := PostLLMHook(nil, response, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotResponse != response || gotErr != nil {
		t.Fatal("PostLLMHook changed the provider response")
	}
}

func TestSharedPluginReloadPreservesReplacementUntilLastCleanup(t *testing.T) {
	_ = Cleanup()
	t.Cleanup(func() {
		_ = Cleanup()
		_ = Cleanup()
	})

	if err := Init(map[string]any{"warmup_requests": float64(1)}); err != nil {
		t.Fatal(err)
	}
	if err := Init(map[string]any{"warmup_requests": float64(2)}); err != nil {
		t.Fatal(err)
	}
	activePlugin.RLock()
	current := activePlugin.instance
	references := activePlugin.references
	activePlugin.RUnlock()
	if current == nil || references != 2 {
		t.Fatalf("after reload: instance=%v references=%d, want an active instance with two loader references", current != nil, references)
	}

	// Bifrost calls the old wrapper's shared Cleanup symbol after replacement.
	if err := Cleanup(); err != nil {
		t.Fatal(err)
	}
	activePlugin.RLock()
	currentAfterOldCleanup := activePlugin.instance
	references = activePlugin.references
	activePlugin.RUnlock()
	if currentAfterOldCleanup != current || references != 1 {
		t.Fatalf("old wrapper cleanup removed the replacement: instance preserved=%t references=%d", currentAfterOldCleanup == current, references)
	}
	if err := Cleanup(); err != nil {
		t.Fatal(err)
	}
	activePlugin.RLock()
	defer activePlugin.RUnlock()
	if activePlugin.instance != nil || activePlugin.references != 0 {
		t.Fatalf("final cleanup left shared state active: instance=%v references=%d", activePlugin.instance != nil, activePlugin.references)
	}
}

func TestSharedPluginRequiresBifrostAIReviewConfiguration(t *testing.T) {
	_ = Cleanup()
	t.Setenv("FRAUDGUARD_AI_REVIEW_ENDPOINT", "")
	t.Setenv("FRAUDGUARD_AI_REVIEW_MODEL", "")
	t.Setenv("FRAUDGUARD_AI_REVIEW_API_KEY", "")
	t.Setenv("FRAUDGUARD_AI_REVIEW_VIRTUAL_KEY_ID", "")

	err := Init(map[string]any{"enable_ai_analysis": true})
	if err == nil || !strings.Contains(err.Error(), "FRAUDGUARD_AI_REVIEW_ENDPOINT") {
		t.Fatalf("Init error = %v, want missing Bifrost AI review configuration", err)
	}
}

func TestSharedPluginInitializesWithBifrostAIReviewConfiguration(t *testing.T) {
	_ = Cleanup()
	t.Cleanup(func() { _ = Cleanup() })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("FRAUDGUARD_AI_REVIEW_ENDPOINT", server.URL+"/v1/chat/completions")
	t.Setenv("FRAUDGUARD_AI_REVIEW_MODEL", "review-model")
	t.Setenv("FRAUDGUARD_AI_REVIEW_API_KEY", "test-only-host-virtual-key")
	t.Setenv("FRAUDGUARD_AI_REVIEW_VIRTUAL_KEY_ID", "test-only-internal-key-id")

	if err := Init(map[string]any{"enable_ai_analysis": true, "warmup_requests": float64(1)}); err != nil {
		t.Fatalf("Init with configured Bifrost AI review: %v", err)
	}
	activePlugin.RLock()
	initialized := activePlugin.instance != nil
	activePlugin.RUnlock()
	if !initialized {
		t.Fatal("plugin was not installed after successful AI adapter setup")
	}
}

func TestCleanupIsSafeBeforeAndAfterInit(t *testing.T) {
	if err := Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := Init(nil); err != nil {
		t.Fatal(err)
	}
	if err := Cleanup(); err != nil {
		t.Fatal(err)
	}
}
