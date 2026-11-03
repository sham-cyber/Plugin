package fraudguard

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func testEngine(t *testing.T, config Config) *Engine {
	t.Helper()
	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return engine
}

func completedRequest(e *Engine, key, provider, model string, at time.Time, input, output int64, failed bool) (Assessment, Assessment) {
	return completedRequestWithCohort(e, key, "", provider, model, at, input, output, failed)
}

func completedRequestWithCohort(e *Engine, key, cohort, provider, model string, at time.Time, input, output int64, failed bool) (Assessment, Assessment) {
	handle, start := e.Start(Request{
		VirtualKeyID: key,
		CohortID:     cohort,
		Provider:     provider,
		Model:        model,
		Timestamp:    at,
	})
	finish := e.Complete(handle, Completion{InputTokens: input, OutputTokens: output, Failed: failed, Timestamp: at})
	return start, finish
}

func TestBenignBaselineStaysQuiet(t *testing.T) {
	engine := testEngine(t, Config{WarmupRequests: 3})
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 8; i++ {
		start, finish := completedRequest(engine, "internal-key-id", "openai", "model-a", base.Add(time.Duration(i)*30*time.Second), 500, 120, false)
		if start.Finding.Score != 0 || finish.Finding.Score != 0 {
			t.Fatalf("benign request %d scored: start=%+v finish=%+v", i, start.Finding, finish.Finding)
		}
	}
}

func TestBelowRateLimitRapidSequenceIsDetected(t *testing.T) {
	engine := testEngine(t, Config{WarmupRequests: 4, AlertThreshold: 30})
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		completedRequest(engine, "internal-key-id", "openai", "model-a", base.Add(time.Duration(i)*30*time.Second), 200, 80, false)
	}
	start, _ := completedRequest(engine, "internal-key-id", "openai", "model-a", base.Add(121*time.Second), 200, 80, false)
	if !containsSignal(start.Finding, "rapid_sequence") || !start.Finding.Alert {
		t.Fatalf("expected below-limit rapid-sequence alert, got %+v", start.Finding)
	}
}

func TestWarmupSuppressesScoring(t *testing.T) {
	engine := testEngine(t, Config{WarmupRequests: 4})
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		completedRequest(engine, "key", "openai", "model-a", base.Add(time.Duration(i)*time.Minute), 100, 40, false)
	}
	start, _ := completedRequest(engine, "key", "new-provider", "new-model", base.Add(3*time.Minute), 100, 40, false)
	if !start.Finding.Warmup || len(start.Finding.Signals) != 0 {
		t.Fatalf("warmup must suppress anomaly signals: %+v", start.Finding)
	}
	next, _ := completedRequest(engine, "key", "third-provider", "third-model", base.Add(4*time.Minute), 100, 40, false)
	if next.Finding.Warmup || !containsSignal(next.Finding, "new_provider") {
		t.Fatalf("expected scoring after warmup, got %+v", next.Finding)
	}
}

func TestScoreAndReasonsAreDeterministic(t *testing.T) {
	e1 := testEngine(t, Config{WarmupRequests: 2})
	e2 := testEngine(t, Config{WarmupRequests: 2})
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for _, engine := range []*Engine{e1, e2} {
		completedRequest(engine, "same-key", "provider-a", "model-a", base, 100, 50, false)
		completedRequest(engine, "same-key", "provider-a", "model-a", base.Add(time.Minute), 100, 50, false)
	}
	a, _ := completedRequest(e1, "same-key", "provider-b", "model-b", base.Add(2*time.Minute), 100, 50, false)
	b, _ := completedRequest(e2, "same-key", "provider-b", "model-b", base.Add(2*time.Minute), 100, 50, false)
	if a.Finding.Score != b.Finding.Score || !reflect.DeepEqual(a.Finding.Signals, b.Finding.Signals) {
		t.Fatalf("detector output changed across identical inputs:\na=%+v\nb=%+v", a.Finding, b.Finding)
	}
	if a.Finding.Score != 50 || a.Finding.Signals[0].Name != "new_model" || a.Finding.Signals[1].Name != "new_provider" {
		t.Fatalf("unexpected explainable score: %+v", a.Finding)
	}
}

func TestTokenSpikeAndRepeatedFailures(t *testing.T) {
	engine := testEngine(t, Config{WarmupRequests: 3, AlertThreshold: 50})
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		completedRequest(engine, "key", "openai", "model-a", base.Add(time.Duration(i)*time.Minute), 300, 100, false)
	}
	_, spike := completedRequest(engine, "key", "openai", "model-a", base.Add(5*time.Minute), 5000, 1200, false)
	if !containsSignal(spike.Finding, "input_volume_spike") || !containsSignal(spike.Finding, "output_volume_spike") {
		t.Fatalf("expected token volume signals, got %+v", spike.Finding)
	}

	for i := 0; i < 10; i++ {
		completedRequest(engine, "failing-key", "openai", "model-a", base.Add(time.Duration(i)*time.Minute), 100, 40, false)
	}
	for i := 0; i < 3; i++ {
		_, finding := completedRequest(engine, "failing-key", "openai", "model-a", base.Add(11*time.Minute+time.Duration(i)*time.Second), 0, 0, true)
		if i < 2 && containsSignal(finding.Finding, "repeated_errors") {
			t.Fatalf("failure signal appeared too early: %+v", finding.Finding)
		}
		if i == 2 && !containsSignal(finding.Finding, "repeated_errors") {
			t.Fatalf("expected repeated error signal: %+v", finding.Finding)
		}
	}
}

func TestConcurrentStartAndComplete(t *testing.T) {
	engine := testEngine(t, Config{WarmupRequests: 5, MaxProfiles: 200, EnableCrossKeyCorrelation: true})
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 75; j++ {
				at := base.Add(time.Duration(i*75+j) * time.Millisecond)
				completedRequestWithCohort(engine, fmt.Sprintf("key-%d", i%4), "authenticated-user", "openai", "model-a", at, 64, 32, false)
			}
		}(i)
	}
	wg.Wait()
	metrics := engine.Metrics()
	if metrics.Requests != 24*75 || metrics.Profiles > 4 {
		t.Fatalf("unexpected concurrent metrics: %+v", metrics)
	}
	if metrics.Cohorts != 1 {
		t.Fatalf("unexpected correlation cohort count: %+v", metrics)
	}
}

func TestProfileExpirationAndMemoryBound(t *testing.T) {
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	engine := testEngine(t, Config{
		ProfileTTL:                time.Minute,
		MaxProfiles:               2,
		EnableCrossKeyCorrelation: true,
	})
	completedRequestWithCohort(engine, "one", "cohort-one", "p", "m", base, 1, 1, false)
	completedRequestWithCohort(engine, "two", "cohort-two", "p", "m", base.Add(time.Second), 1, 1, false)
	completedRequestWithCohort(engine, "three", "cohort-three", "p", "m", base.Add(2*time.Second), 1, 1, false)
	if got := engine.Metrics().Profiles; got != 2 {
		t.Fatalf("profile limit not enforced: %d", got)
	}
	if got := engine.Metrics().Cohorts; got != 2 {
		t.Fatalf("cohort memory bound not enforced: %d", got)
	}
	if removed := engine.Prune(base.Add(10 * time.Minute)); removed != 4 {
		t.Fatalf("expected profiles and cohorts to expire, got %d records", removed)
	}
	if metrics := engine.Metrics(); metrics.Profiles != 0 || metrics.Cohorts != 0 {
		t.Fatalf("state remains after expiration: %+v", metrics)
	}
}

func TestFindingsNeverContainPromptOrKey(t *testing.T) {
	const prompt = "PRIVATE_PROMPT_should_never_appear"
	const secretKey = "PRIVATE_VIRTUAL_KEY_should_never_appear"
	var findings []Finding
	engine := testEngine(t, Config{
		WarmupRequests:             1,
		EnableCrossKeyCorrelation:  true,
		MaxDistinctKeysPerIdentity: 1,
		OnFinding:                  func(f Finding) { findings = append(findings, f) },
	})
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	completedRequestWithCohort(engine, secretKey, "PRIVATE_COHORT_should_never_appear", "provider-a", "model-a", base, 10, 10, false)
	completedRequestWithCohort(engine, secretKey+"-second", "PRIVATE_COHORT_should_never_appear", "provider-b", "model-b", base.Add(time.Second), 10, 10, false)
	encoded := fmt.Sprintf("%+v", findings)
	if strings.Contains(encoded, prompt) || strings.Contains(encoded, secretKey) || strings.Contains(encoded, "PRIVATE_COHORT_should_never_appear") {
		t.Fatalf("finding leaked protected values: %s", encoded)
	}
	for _, finding := range findings {
		for _, signal := range finding.Signals {
			if strings.Contains(signal.Reason, "provider-b") || strings.Contains(signal.Reason, "model-b") {
				t.Fatalf("finding leaked a raw model/provider label: %+v", signal)
			}
		}
	}
	for cohortID := range engine.cohorts {
		if strings.Contains(cohortID, "PRIVATE_COHORT_should_never_appear") {
			t.Fatal("cohort state retained a clear-text identity")
		}
		for keyID := range engine.cohorts[cohortID].keys {
			if keyID == secretKey || keyID == secretKey+"-second" {
				t.Fatal("cohort state retained a clear-text virtual-key ID")
			}
		}
	}
	for profileID := range engine.profiles {
		if profileID == secretKey || profileID == secretKey+"-second" {
			t.Fatal("profile state retained a clear-text virtual-key ID")
		}
	}
}

func TestCrossKeyCorrelationIsOptInAndHashed(t *testing.T) {
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	disabled := testEngine(t, Config{MaxDistinctKeysPerIdentity: 1})
	completedRequestWithCohort(disabled, "vk-one", "trusted-user", "p", "m", base, 1, 1, false)
	startDisabled, _ := completedRequestWithCohort(disabled, "vk-two", "trusted-user", "p", "m", base.Add(time.Second), 1, 1, false)
	if containsSignal(startDisabled.Finding, "key_rotation") || disabled.Metrics().Cohorts != 0 {
		t.Fatalf("cross-key state should be absent unless enabled: %+v", disabled.Metrics())
	}

	enabled := testEngine(t, Config{
		WarmupRequests:             1,
		AlertThreshold:             30,
		EnableCrossKeyCorrelation:  true,
		MaxDistinctKeysPerIdentity: 2,
	})
	for i := 0; i < 3; i++ {
		start, _ := completedRequestWithCohort(enabled, fmt.Sprintf("vk-%d", i), "trusted-user", "p", "m", base.Add(time.Duration(i)*time.Second), 1, 1, false)
		if i < 2 && containsSignal(start.Finding, "key_rotation") {
			t.Fatalf("rotation signal appeared before configured limit: %+v", start.Finding)
		}
		if i == 2 && (!containsSignal(start.Finding, "key_rotation") || !start.Finding.Alert) {
			t.Fatalf("expected opt-in rotation alert: %+v", start.Finding)
		}
	}
	repeat, _ := completedRequestWithCohort(enabled, "vk-2", "trusted-user", "p", "m", base.Add(4*time.Second), 1, 1, false)
	if containsSignal(repeat.Finding, "key_rotation") {
		t.Fatalf("same key should not repeatedly raise a rotation signal: %+v", repeat.Finding)
	}
	for id := range enabled.cohorts {
		if id == "trusted-user" {
			t.Fatal("cohort ID was stored in clear text")
		}
		for keyID := range enabled.cohorts[id].keys {
			if strings.HasPrefix(keyID, "vk-") {
				t.Fatal("virtual-key ID was stored in clear text")
			}
		}
	}
}

func TestExemptionsAndAlertThreshold(t *testing.T) {
	engine := testEngine(t, Config{
		WarmupRequests:      1,
		AlertThreshold:      60,
		ExemptVirtualKeyIDs: []string{"exempt-internal-id"},
	})
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	completedRequest(engine, "exempt-internal-id", "p1", "m1", base, 1, 1, false)
	completedRequest(engine, "exempt-internal-id", "p2", "m2", base.Add(time.Second), 1, 1, false)
	if engine.Metrics().ExemptRequests != 2 || engine.Metrics().Profiles != 0 {
		t.Fatalf("exemption did not skip profile state: %+v", engine.Metrics())
	}
	if len(engine.cfg.ExemptVirtualKeyIDs) != 0 {
		t.Fatal("engine retained clear-text exemption IDs")
	}

	other := testEngine(t, Config{WarmupRequests: 1, AlertThreshold: 60})
	completedRequest(other, "key", "p1", "m1", base, 1, 1, false)
	finding, _ := completedRequest(other, "key", "p2", "m2", base.Add(time.Minute), 1, 1, false)
	if finding.Finding.Score != 50 || finding.Finding.Alert {
		t.Fatalf("threshold should suppress alert without hiding finding: %+v", finding.Finding)
	}
}

func TestMissingIdentityDoesNotCreateSharedProfile(t *testing.T) {
	engine := testEngine(t, Config{})
	handle, _ := engine.Start(Request{Provider: "openai", Model: "model-a"})
	if handle.profileID != "" || engine.Metrics().Profiles != 0 || engine.Metrics().Requests != 0 {
		t.Fatalf("missing identity should not create a profile: handle=%+v metrics=%+v", handle, engine.Metrics())
	}
}

func containsSignal(finding Finding, name string) bool {
	for _, signal := range finding.Signals {
		if signal.Name == name {
			return true
		}
	}
	return false
}
