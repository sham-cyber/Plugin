// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

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
