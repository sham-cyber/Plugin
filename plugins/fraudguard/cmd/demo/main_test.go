// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"
	"time"

	"github.com/maximhq/bifrost/plugins/fraudguard"
)

func TestBuildDemoReportUsesFraudGuardEngine(t *testing.T) {
	report, _, err := buildDemoReport()
	if err != nil {
		t.Fatalf("build demo report: %v", err)
	}
	if report.Source != "fraudguard-go-engine" || !report.Synthetic {
		t.Fatalf("unexpected demo source: %+v", report)
	}
	if len(report.Cases) != 4 {
		t.Fatalf("expected four engine-backed demo cases, got %d", len(report.Cases))
	}

	want := map[string]struct {
		score  int
		alert  bool
		signal string
	}{
		"behavior":      {score: 30, alert: false, signal: "rapid_sequence"},
		"payment":       {score: 95, alert: true, signal: "payment_velocity"},
		"key_status":    {score: 100, alert: true, signal: "key_expired"},
		"context_guard": {score: 100, alert: true, signal: "context_progressive_jailbreak"},
	}
	for _, result := range report.Cases {
		expected, ok := want[result.ID]
		if !ok {
			t.Fatalf("unexpected demo case %q", result.ID)
		}
		if result.Finding.Score != expected.score || result.Finding.Alert != expected.alert {
			t.Errorf("%s finding = score %d alert %t; want score %d alert %t",
				result.ID, result.Finding.Score, result.Finding.Alert, expected.score, expected.alert)
		}
		foundSignal := false
		for _, signal := range result.Finding.Signals {
			if signal.Name == expected.signal {
				foundSignal = true
			}
		}
		if !foundSignal {
			t.Errorf("%s finding missing signal %q: %+v", result.ID, expected.signal, result.Finding.Signals)
		}
	}
}

func TestBuildDemoReportCanUseOptionalAIReview(t *testing.T) {
	reviews := make(chan fraudguard.AIReviewEvent, 1)
	analyzer := fraudguard.AIAnalyzerFunc(func(_ context.Context, input fraudguard.AIAnalysisInput) (fraudguard.AIAnalysisResult, error) {
		if input.Score != 30 || len(input.Signals) == 0 {
			t.Errorf("AI adapter received an unexpected synthetic finding: %+v", input)
		}
		return fraudguard.AIAnalysisResult{
			Conclusion:  fraudguard.AIConclusionReviewSuggested,
			Confidence:  70,
			Explanation: "Synthetic behavior signals merit review.",
		}, nil
	})
	report, _, err := buildDemoReportWithAI(analyzer, "demo-review-key-id", func(event fraudguard.AIReviewEvent) {
		reviews <- event
	})
	if err != nil {
		t.Fatalf("build AI demo report: %v", err)
	}
	select {
	case event := <-reviews:
		if event.Analysis.Conclusion != fraudguard.AIConclusionReviewSuggested {
			t.Fatalf("unexpected AI advisory: %+v", event.Analysis)
		}
	case <-time.After(time.Second):
		t.Fatal("synthetic demo did not invoke the optional AI adapter")
	}
	if len(report.Cases) != 4 || report.Cases[0].Finding.Score != 30 {
		t.Fatalf("AI review changed the deterministic demo findings: %+v", report.Cases)
	}
}
