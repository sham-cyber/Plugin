// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAIReviewIsOptInPrivacySafeAndAdvisory(t *testing.T) {
	const (
		key      = "PRIVATE_VIRTUAL_KEY_VALUE"
		provider = "PRIVATE_PROVIDER_LABEL"
		model    = "PRIVATE_MODEL_LABEL"
	)
	inputs := make(chan AIAnalysisInput, 2)
	reviews := make(chan AIReviewEvent, 2)
	engine, err := NewEngine(Config{
		WarmupRequests:     1,
		AlertThreshold:     30,
		EnableAIAnalysis:   true,
		AIAnalysisMinScore: 30,
		AIAnalyzer: AIAnalyzerFunc(func(_ context.Context, input AIAnalysisInput) (AIAnalysisResult, error) {
			inputs <- input
			return AIAnalysisResult{
				Conclusion:  AIConclusionReviewSuggested,
				Confidence:  80,
				Explanation: "Review the combined behavior signals.",
			}, nil
		}),
		OnAIReview: func(event AIReviewEvent) { reviews <- event },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	engine.Start(Request{
		VirtualKeyID: key,
		Provider:     "provider-baseline",
		Model:        "model-baseline",
		Timestamp:    base,
	})
	_, finding := engine.Start(Request{
		VirtualKeyID: key,
		Provider:     provider,
		Model:        model,
		Timestamp:    base.Add(time.Minute),
	})
	if finding.Finding.Score != 50 || !finding.Finding.Alert {
		t.Fatalf("AI analysis changed or replaced deterministic scoring: %+v", finding.Finding)
	}

	var input AIAnalysisInput
	select {
	case input = <-inputs:
	case <-time.After(time.Second):
		t.Fatal("host model adapter was not called")
	}
	var review AIReviewEvent
	select {
	case review = <-reviews:
	case <-time.After(time.Second):
		t.Fatal("AI review callback was not called")
	}
	if input.Score != finding.Finding.Score || input.Alert != finding.Finding.Alert {
		t.Fatalf("AI adapter received a different base finding: input=%+v finding=%+v", input, finding.Finding)
	}
	if review.Finding.Score != finding.Finding.Score || !review.Finding.Alert {
		t.Fatalf("AI advice changed the original finding: %+v", review)
	}
	if review.Analysis.Conclusion != AIConclusionReviewSuggested || review.Analysis.Confidence != 80 {
		t.Fatalf("unexpected AI advice: %+v", review.Analysis)
	}
	encoded := fmt.Sprintf("%+v %+v", input, review)
	for _, privateValue := range []string{key, provider, model} {
		if strings.Contains(encoded, privateValue) {
			t.Fatalf("AI request or review leaked %q: %s", privateValue, encoded)
		}
	}
	if got := engine.Metrics(); got.AIReviewsQueued != 1 || got.AIReviewsCompleted != 1 || got.AIReviewsFailed != 0 {
		t.Fatalf("unexpected AI review metrics: %+v", got)
	}
}

func TestTenantPlanCanDisableOptionalAIReview(t *testing.T) {
	called := make(chan struct{}, 1)
	engine, err := NewEngine(Config{
		WarmupRequests:     1,
		AlertThreshold:     30,
		EnableAIAnalysis:   true,
		AIAnalysisMinScore: 30,
		AIAnalyzer: AIAnalyzerFunc(func(context.Context, AIAnalysisInput) (AIAnalysisResult, error) {
			called <- struct{}{}
			return AIAnalysisResult{Conclusion: AIConclusionNoAddedConcern}, nil
		}),
		OnAIReview: func(AIReviewEvent) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	disabled := false
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	engine.Start(Request{VirtualKeyID: "tenant-key", Provider: "p1", Model: "m1", Timestamp: base, AIReviewOverride: &disabled})
	engine.Start(Request{VirtualKeyID: "tenant-key", Provider: "p2", Model: "m2", Timestamp: base.Add(time.Minute), AIReviewOverride: &disabled})
	if metrics := engine.Metrics(); metrics.AIReviewsQueued != 0 {
		t.Fatalf("plan-disabled AI review was still queued: %+v", metrics)
	}
	select {
	case <-called:
		t.Fatal("plan-disabled AI adapter was called")
	case <-time.After(30 * time.Millisecond):
	}
}

func TestAIAnalysisDisabledByDefault(t *testing.T) {
	called := make(chan struct{}, 1)
	engine := testEngine(t, Config{
		WarmupRequests: 1,
		AlertThreshold: 30,
		AIAnalyzer: AIAnalyzerFunc(func(context.Context, AIAnalysisInput) (AIAnalysisResult, error) {
			called <- struct{}{}
			return AIAnalysisResult{Conclusion: AIConclusionInsufficientData, Explanation: "No extra context."}, nil
		}),
		OnAIReview: func(AIReviewEvent) { called <- struct{}{} },
	})
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	engine.Start(Request{VirtualKeyID: "key", Provider: "p1", Model: "m1", Timestamp: base})
	engine.Start(Request{VirtualKeyID: "key", Provider: "p2", Model: "m2", Timestamp: base.Add(time.Minute)})
	if got := engine.Metrics(); got.AIReviewsQueued != 0 || got.AIReviewsCompleted != 0 {
		t.Fatalf("AI analysis should be disabled by default: %+v", got)
	}
	select {
	case <-called:
		t.Fatal("disabled AI analyzer was called")
	default:
	}
}

func TestAIAnalysisRequiresHostAdapterAndCallback(t *testing.T) {
	if _, err := NewEngine(Config{EnableAIAnalysis: true, OnAIReview: func(AIReviewEvent) {}}); err == nil {
		t.Fatal("expected an error when the host model adapter is missing")
	}
	if _, err := NewEngine(Config{
		EnableAIAnalysis: true,
		AIAnalyzer: AIAnalyzerFunc(func(context.Context, AIAnalysisInput) (AIAnalysisResult, error) {
			return AIAnalysisResult{}, nil
		}),
	}); err == nil {
		t.Fatal("expected an error when the AI review callback is missing")
	}
}

func TestAIQueueIsBoundedAndDoesNotWaitForModel(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	engine, err := NewEngine(Config{
		WarmupRequests:      1,
		AlertThreshold:      30,
		EnableAIAnalysis:    true,
		AIAnalysisMinScore:  30,
		AIAnalysisWorkers:   1,
		AIAnalysisQueueSize: 1,
		AIAnalyzer: AIAnalyzerFunc(func(ctx context.Context, _ AIAnalysisInput) (AIAnalysisResult, error) {
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-release:
				return AIAnalysisResult{
					Conclusion:  AIConclusionReviewSuggested,
					Confidence:  70,
					Explanation: "Review the behavior signals.",
				}, nil
			case <-ctx.Done():
				return AIAnalysisResult{}, ctx.Err()
			}
		}),
		OnAIReview: func(AIReviewEvent) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	engine.Start(Request{VirtualKeyID: "key", Provider: "p0", Model: "m0", Timestamp: base})
	engine.Start(Request{VirtualKeyID: "key", Provider: "p1", Model: "m1", Timestamp: base.Add(time.Minute)})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("AI worker did not start")
	}
	engine.Start(Request{VirtualKeyID: "key", Provider: "p2", Model: "m2", Timestamp: base.Add(2 * time.Minute)})
	engine.Start(Request{VirtualKeyID: "key", Provider: "p3", Model: "m3", Timestamp: base.Add(3 * time.Minute)})
	if got := engine.Metrics().AIReviewsDropped; got != 1 {
		t.Fatalf("expected one dropped review on a full queue, got %d", got)
	}
	close(release)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if got := engine.Metrics(); got.AIReviewsQueued != 2 || got.AIReviewsCompleted != 2 {
		t.Fatalf("unexpected bounded queue metrics after drain: %+v", got)
	}
}

func TestAIAdapterFailureDoesNotChangeFinding(t *testing.T) {
	engine, err := NewEngine(Config{
		WarmupRequests:   1,
		AlertThreshold:   30,
		EnableAIAnalysis: true,
		AIAnalyzer: AIAnalyzerFunc(func(context.Context, AIAnalysisInput) (AIAnalysisResult, error) {
			return AIAnalysisResult{}, errors.New("private provider error")
		}),
		OnAIReview: func(AIReviewEvent) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	engine.Start(Request{VirtualKeyID: "key", Provider: "p1", Model: "m1", Timestamp: base})
	_, finding := engine.Start(Request{VirtualKeyID: "key", Provider: "p2", Model: "m2", Timestamp: base.Add(time.Minute)})
	if finding.Finding.Score != 50 || !finding.Finding.Alert {
		t.Fatalf("AI adapter failure changed the deterministic finding: %+v", finding.Finding)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if got := engine.Metrics(); got.AIReviewsFailed != 1 || got.AIReviewsCompleted != 0 {
		t.Fatalf("adapter error should be counted without exposing error text: %+v", got)
	}
}

func TestAIPanicsAreContainedAndCounted(t *testing.T) {
	tests := []struct {
		name     string
		analyzer AIAnalyzer
		callback func(AIReviewEvent)
	}{
		{
			name: "analyzer",
			analyzer: AIAnalyzerFunc(func(context.Context, AIAnalysisInput) (AIAnalysisResult, error) {
				panic("private analyzer failure")
			}),
			callback: func(AIReviewEvent) {},
		},
		{
			name: "review callback",
			analyzer: AIAnalyzerFunc(func(context.Context, AIAnalysisInput) (AIAnalysisResult, error) {
				return AIAnalysisResult{
					Conclusion:  AIConclusionReviewSuggested,
					Confidence:  50,
					Explanation: "Review the behavior summary.",
				}, nil
			}),
			callback: func(AIReviewEvent) {
				panic("private callback failure")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, err := NewEngine(Config{
				WarmupRequests:     1,
				EnableAIAnalysis:   true,
				AIAnalysisMinScore: 1,
				AIAnalyzer:         test.analyzer,
				OnAIReview:         test.callback,
			})
			if err != nil {
				t.Fatal(err)
			}
			base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
			engine.Start(Request{VirtualKeyID: "key", Provider: "p1", Model: "m1", Timestamp: base})
			engine.Start(Request{VirtualKeyID: "key", Provider: "p2", Model: "m2", Timestamp: base.Add(time.Minute)})
			if err := engine.Close(); err != nil {
				t.Fatal(err)
			}
			if got := engine.Metrics(); got.AIReviewsFailed != 1 || got.AIReviewsCompleted != 0 {
				t.Fatalf("AI panic was not contained and counted: %+v", got)
			}
		})
	}
}

func TestConcurrentAIReviewsAreSafe(t *testing.T) {
	const workers = 20
	var reviews sync.WaitGroup
	reviews.Add(workers)
	engine, err := NewEngine(Config{
		WarmupRequests:      1,
		AlertThreshold:      30,
		EnableAIAnalysis:    true,
		AIAnalysisMinScore:  30,
		AIAnalysisWorkers:   4,
		AIAnalysisQueueSize: workers,
		AIAnalyzer: AIAnalyzerFunc(func(_ context.Context, input AIAnalysisInput) (AIAnalysisResult, error) {
			if input.Score < input.Threshold || len(input.Signals) == 0 {
				return AIAnalysisResult{}, errors.New("invalid sanitized summary")
			}
			return AIAnalysisResult{
				Conclusion:  AIConclusionInsufficientData,
				Confidence:  50,
				Explanation: "The behavior summary is incomplete.",
			}, nil
		}),
		OnAIReview: func(AIReviewEvent) { reviews.Done() },
	})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	for i := 0; i < workers; i++ {
		key := fmt.Sprintf("key-%d", i)
		engine.Start(Request{VirtualKeyID: key, Provider: "baseline", Model: "baseline", Timestamp: base})
		engine.Start(Request{
			VirtualKeyID: key,
			Provider:     fmt.Sprintf("provider-%d", i),
			Model:        fmt.Sprintf("model-%d", i),
			Timestamp:    base.Add(time.Minute),
		})
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		reviews.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("AI review workers did not drain all queued reviews")
	}
	if got := engine.Metrics(); got.AIReviewsQueued != workers || got.AIReviewsCompleted != workers || got.AIReviewsDropped != 0 {
		t.Fatalf("unexpected concurrent AI review metrics: %+v", got)
	}
}

func TestAIResultIsBoundedAndValidatesConclusion(t *testing.T) {
	explanation := strings.Repeat("x", maxAIExplanationRunes+20)
	got, ok := normalizeAIAnalysisResult(AIAnalysisResult{
		Conclusion:  AIConclusionNoAddedConcern,
		Confidence:  100,
		Explanation: explanation,
	})
	if !ok || len([]rune(got.Explanation)) != maxAIExplanationRunes {
		t.Fatalf("AI explanation was not bounded: ok=%v length=%d", ok, len([]rune(got.Explanation)))
	}
	if _, ok := normalizeAIAnalysisResult(AIAnalysisResult{Conclusion: "block", Confidence: 100, Explanation: "Reject request."}); ok {
		t.Fatal("unsupported AI action should be rejected")
	}
}
