// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func validRuntimeConfigDocument() RuntimeConfigDocument {
	return RuntimeConfigDocument{
		Version:                     1,
		AlertThreshold:              60,
		WarmupRequests:              2,
		Window:                      "15m",
		ProfileTTL:                  "2h",
		MaxProfiles:                 100,
		MaxEventsPerProfile:         32,
		EnableCrossKeyCorrelation:   false,
		MaxDistinctKeysPerIdentity:  3,
		CorrelationWindow:           "15m",
		EnablePaymentVelocity:       false,
		EnablePaymentLocation:       false,
		PaymentWindow:               "15m",
		PaymentProfileTTL:           "2h",
		MaxPaymentSubjects:          100,
		MaxPaymentEventsPerSubject:  32,
		MaxPaymentsPerWindow:        10,
		PaymentLocationWarmupEvents: 3,
		EnableExpiredKeySignal:      false,
		EnableRevokedKeySignal:      false,
		EnableExposedKeySignal:      false,
		EnablePromptOverrideSignals: false,
		EnablePromptLengthSignal:    false,
		MaxPromptBytes:              32_768,
		MaxPromptScanBytes:          64 * 1024,
		EnableContextGuard:          false,
		MaxContextMessages:          defaultContextMessages,
		MaxContextScanBytes:         defaultContextScanBytes,
	}
}

func marshalRuntimeConfig(t *testing.T, document RuntimeConfigDocument) []byte {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestConfigReloaderAppliesValidConfigAndPreservesLastGood(t *testing.T) {
	engine, err := NewEngine(Config{AlertThreshold: 80, WarmupRequests: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	reloader, err := NewConfigReloader(engine)
	if err != nil {
		t.Fatal(err)
	}

	document := validRuntimeConfigDocument()
	document.AlertThreshold = 80
	document.EnableRevokedKeySignal = true
	document.EnableContextGuard = true
	document.ExemptVirtualKeyIDs = []string{"internal-exempt-id"}
	if err := reloader.ReloadJSON(marshalRuntimeConfig(t, document)); err != nil {
		t.Fatal(err)
	}
	_, assessment := engine.Start(Request{
		VirtualKeyID:     "key-a",
		CredentialStatus: CredentialStatus{Revoked: true},
	})
	if assessment.Finding.Score != 70 || assessment.Finding.Alert {
		t.Fatalf("expected a non-alerting revoked-key signal at threshold 80: %+v", assessment.Finding)
	}

	document.AlertThreshold = 60
	if err := reloader.ReloadJSON(marshalRuntimeConfig(t, document)); err != nil {
		t.Fatal(err)
	}
	_, assessment = engine.Start(Request{
		VirtualKeyID:     "key-b",
		CredentialStatus: CredentialStatus{Revoked: true},
	})
	if assessment.Finding.Score != 70 || !assessment.Finding.Alert {
		t.Fatalf("updated threshold did not apply: %+v", assessment.Finding)
	}

	_, _ = engine.Start(Request{VirtualKeyID: "internal-exempt-id"})
	metrics := engine.Metrics()
	if metrics.ExemptRequests != 1 || metrics.Profiles != 2 {
		t.Fatalf("updated exemption was not applied: %+v", metrics)
	}
	_, contextAssessment := engine.Start(Request{
		VirtualKeyID:            "context-config-key",
		ContextDataExfiltration: true,
	})
	if !containsSignal(contextAssessment.Finding, "context_data_exfiltration") {
		t.Fatalf("runtime context-guard setting was not applied: %+v", contextAssessment.Finding)
	}
	if strings.Contains(string(mustJSON(t, reloader.Status())), "internal-exempt-id") {
		t.Fatal("config status exposed an exemption ID")
	}
	for digest := range engine.exemptions {
		if digest == "internal-exempt-id" {
			t.Fatal("engine retained an exemption ID in clear text")
		}
	}

	generation := reloader.Status().Generation
	document.AlertThreshold = 101
	if err := reloader.ReloadJSON(marshalRuntimeConfig(t, document)); err == nil {
		t.Fatal("invalid threshold was accepted")
	}
	status := reloader.Status()
	if status.Generation != generation || !status.LastAttemptFail || status.LastErrorCode != "invalid_config" ||
		engine.configSnapshot().AlertThreshold != 60 {
		t.Fatalf("invalid reload changed active config or status: status=%+v config=%+v", status, engine.configSnapshot())
	}
}

func TestConfigReloaderRejectsUnknownBlockingConfigWithoutChangingState(t *testing.T) {
	engine, err := NewEngine(Config{AlertThreshold: 60})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	reloader, err := NewConfigReloader(engine)
	if err != nil {
		t.Fatal(err)
	}
	document := validRuntimeConfigDocument()
	if err := reloader.ReloadJSON(marshalRuntimeConfig(t, document)); err != nil {
		t.Fatal(err)
	}

	invalid := strings.TrimSuffix(string(marshalRuntimeConfig(t, document)), "}") + `,"blocklist":["key-to-block"]}`
	if err := reloader.ReloadJSON([]byte(invalid)); err == nil {
		t.Fatal("unknown blocklist field was accepted")
	}
	if engine.configSnapshot().AlertThreshold != 60 || !reloader.Status().LastAttemptFail {
		t.Fatal("unknown blocking field changed live config")
	}
}

func TestPluginConfigWatcherReloadsAndStopsOnCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fraudguard.json")
	document := validRuntimeConfigDocument()
	document.AlertThreshold = 35
	if err := os.WriteFile(path, marshalRuntimeConfig(t, document), 0o600); err != nil {
		t.Fatal(err)
	}
	plugin, err := New(Config{AlertThreshold: 70})
	if err != nil {
		t.Fatal(err)
	}
	if err := plugin.startConfigWatcher(path, 100*time.Millisecond); err != nil {
		_ = plugin.Cleanup()
		t.Fatal(err)
	}
	if got := plugin.Engine().configSnapshot().AlertThreshold; got != 35 {
		t.Fatalf("initial config file was not loaded: threshold=%d", got)
	}

	document.AlertThreshold = 45
	if err := os.WriteFile(path, marshalRuntimeConfig(t, document), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for plugin.ConfigReloadStatus().Generation < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := plugin.Engine().configSnapshot().AlertThreshold; got != 45 {
		t.Fatalf("updated config file was not applied: threshold=%d status=%+v", got, plugin.ConfigReloadStatus())
	}

	if err := plugin.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigReloadCanRunConcurrentlyWithRequests(t *testing.T) {
	engine, err := NewEngine(Config{WarmupRequests: 1, AlertThreshold: 60})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	reloader, err := NewConfigReloader(engine)
	if err != nil {
		t.Fatal(err)
	}
	document := validRuntimeConfigDocument()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			document.AlertThreshold = 40 + i%50
			data, marshalErr := json.Marshal(document)
			if marshalErr != nil {
				t.Error(marshalErr)
				return
			}
			if reloadErr := reloader.ReloadJSON(data); reloadErr != nil {
				t.Error(reloadErr)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			engine.Start(Request{VirtualKeyID: "concurrent-key", Provider: "p", Model: "m"})
		}
	}()
	wg.Wait()
}

func TestConfigReloaderRejectsMissingAndOversizedFiles(t *testing.T) {
	engine, err := NewEngine(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	reloader, err := NewConfigReloader(engine)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloader.ReloadFile(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing config file was accepted")
	}
	path := filepath.Join(t.TempDir(), "large.json")
	if err := os.WriteFile(path, make([]byte, maxRuntimeConfigBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := reloader.ReloadFile(path); err == nil {
		t.Fatal("oversized config file was accepted")
	}
	if reloader.Status().Generation != 0 || !reloader.Status().LastAttemptFail {
		t.Fatalf("failed initial loads should not create a generation: %+v", reloader.Status())
	}
}

func TestConfigExampleIsAValidRuntimeSnapshot(t *testing.T) {
	data, err := os.ReadFile("config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	reloader, err := NewConfigReloader(engine)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloader.ReloadJSON(data); err != nil {
		t.Fatalf("config.example.json is invalid: %v", err)
	}
	if reloader.Status().Generation != 1 {
		t.Fatalf("example config was not applied: %+v", reloader.Status())
	}
}

func TestConfigWatcherReportsAndRecoversFromInvalidFile(t *testing.T) {
	engine, err := NewEngine(Config{AlertThreshold: 60})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	reloader, err := NewConfigReloader(engine)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fraudguard.json")
	document := validRuntimeConfigDocument()
	if err := os.WriteFile(path, marshalRuntimeConfig(t, document), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errorsOut, err := reloader.WatchFile(ctx, path, 100*time.Millisecond)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() {
		cancel()
		for range errorsOut {
		}
	}()

	if err := os.WriteFile(path, []byte(`{"version":1,"alert_threshold":999}`), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !reloader.Status().LastAttemptFail && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !reloader.Status().LastAttemptFail || engine.configSnapshot().AlertThreshold != 60 {
		t.Fatalf("invalid watcher update was not rejected: status=%+v config=%+v", reloader.Status(), engine.configSnapshot())
	}

	document.AlertThreshold = 45
	if err := os.WriteFile(path, marshalRuntimeConfig(t, document), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for reloader.Status().Generation < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if reloader.Status().Generation != 2 || reloader.Status().LastAttemptFail || engine.configSnapshot().AlertThreshold != 45 {
		t.Fatalf("watcher did not recover after a valid update: status=%+v config=%+v", reloader.Status(), engine.configSnapshot())
	}
}
