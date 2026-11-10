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

func TestBudgetMonitorWarnsAndReportsOveragesWithoutBlocking(t *testing.T) {
	monitor, err := NewBudgetMonitor(BudgetMonitorConfig{MaxEntries: 8, Retention: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer plugin.Cleanup()
	if err := plugin.SetBudgetMonitor(monitor); err != nil {
		t.Fatal(err)
	}

	warning, err := plugin.ObserveBudget(BudgetEvent{
		TenantID: "sensitive-tenant-id", Period: "2026-11",
		Tokens: 80, EstimatedCostMicros: 80,
		MaxTokens: 100, MaxCostMicros: 100, WarningPercent: 80,
	})
	if err != nil || warning.Outcome != "budget_warning_observe_only" ||
		!warning.TokenWarning || !warning.CostWarning || warning.Exceeded {
		t.Fatalf("expected advisory warning at the configured threshold: result=%+v err=%v", warning, err)
	}
	overage, err := plugin.ObserveBudget(BudgetEvent{
		TenantID: "sensitive-tenant-id", Period: "2026-11",
		Tokens: 30, EstimatedCostMicros: 50,
		MaxTokens: 100, MaxCostMicros: 100,
	})
	if err != nil || overage.Outcome != "budget_exceeded_observe_only" ||
		!overage.TokenLimitExceeded || !overage.CostLimitExceeded {
		t.Fatalf("expected observe-only overage: result=%+v err=%v", overage, err)
	}
	encoded, err := json.Marshal(overage)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "sensitive-tenant-id") || strings.Contains(string(encoded), "blocked") {
		t.Fatalf("budget assessment leaked tenant identity or blocking state: %s", encoded)
	}
	if metrics := plugin.BudgetMetrics(); metrics.Observations != 2 || metrics.Warnings != 1 ||
		metrics.Exceeded != 1 || metrics.TrackedPeriods != 1 {
		t.Fatalf("unexpected aggregate budget metrics: %+v", metrics)
	}
}

func TestBudgetMonitorBoundsStateAndExpiresEntries(t *testing.T) {
	monitor, err := NewBudgetMonitor(BudgetMonitorConfig{MaxEntries: 1, Retention: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := monitor.Observe(BudgetEvent{
		TenantID: "tenant-a", Period: "2026-11", Tokens: 4,
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	if removed := monitor.Prune(time.Now()); removed != 1 || monitor.Metrics().TrackedPeriods != 0 {
		t.Fatalf("expired budget aggregate was not removed: removed=%d metrics=%+v", removed, monitor.Metrics())
	}
	if _, err := monitor.Observe(BudgetEvent{
		TenantID: "tenant-b", Period: "2026-11", Tokens: 7,
	}); err != nil {
		t.Fatal(err)
	}
	if monitor.Metrics().TrackedPeriods != 1 {
		t.Fatalf("state bound was not respected: %+v", monitor.Metrics())
	}
}

func TestBudgetMonitorConcurrentObservationsAreSafe(t *testing.T) {
	monitor, err := NewBudgetMonitor(BudgetMonitorConfig{MaxEntries: 4})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 12 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 100 {
				if _, err := monitor.Observe(BudgetEvent{
					TenantID: "trusted-tenant", Period: "2026-11",
					Tokens: 1, EstimatedCostMicros: 1,
				}); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	workers.Wait()
	result, err := monitor.Observe(BudgetEvent{
		TenantID: "trusted-tenant", Period: "2026-11",
		MaxTokens: 1200, MaxCostMicros: 1200,
	})
	if err != nil || result.TokensUsed != 1200 || result.EstimatedCostMicros != 1200 {
		t.Fatalf("concurrent budget totals are incorrect: result=%+v err=%v", result, err)
	}
}
