// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestOverageNotifierDeduplicatesThresholdStagesAndBoundsState(t *testing.T) {
	notifier, err := NewOverageNotifier(OverageNotifierConfig{MaxEntries: 2, Retention: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := notifier.ClaimOverageAlert(ctx, "private-company-id", "2026-11", PlanPro, OverageStageNearLimit)
	if err != nil || !first {
		t.Fatalf("first near-limit claim should be accepted: first=%t err=%v", first, err)
	}
	second, err := notifier.ClaimOverageAlert(ctx, "private-company-id", "2026-11", PlanPro, OverageStageNearLimit)
	if err != nil || second {
		t.Fatalf("duplicate near-limit claim should be suppressed: second=%t err=%v", second, err)
	}
	exceeded, err := notifier.ClaimOverageAlert(ctx, "private-company-id", "2026-11", PlanPro, OverageStageExceeded)
	if err != nil || !exceeded {
		t.Fatalf("the exceeded stage should be independently claimable: claimed=%t err=%v", exceeded, err)
	}
	for key := range notifier.claimed {
		if strings.Contains(key, "private-company-id") {
			t.Fatalf("notifier retained the raw tenant identifier: %q", key)
		}
	}
	if removed := notifier.Prune(time.Now().Add(2 * time.Hour)); removed != 2 {
		t.Fatalf("expected bounded alert state to expire, removed=%d", removed)
	}
}

func TestRedisProductStoreDeduplicatesAndReleasesOverageAlerts(t *testing.T) {
	store, server := newTestRedisProductStore(t)
	ctx := context.Background()
	claim, err := store.ClaimOverageAlert(ctx, "private-company-id", "2026-11", PlanEnterprise, OverageStageNearLimit)
	if err != nil || !claim {
		t.Fatalf("first Redis alert claim failed: claim=%t err=%v", claim, err)
	}
	claim, err = store.ClaimOverageAlert(ctx, "private-company-id", "2026-11", PlanEnterprise, OverageStageNearLimit)
	if err != nil || claim {
		t.Fatalf("duplicate Redis alert claim was not suppressed: claim=%t err=%v", claim, err)
	}
	if err := store.ReleaseOverageAlert(ctx, "private-company-id", "2026-11", PlanEnterprise, OverageStageNearLimit); err != nil {
		t.Fatal(err)
	}
	claim, err = store.ClaimOverageAlert(ctx, "private-company-id", "2026-11", PlanEnterprise, OverageStageNearLimit)
	if err != nil || !claim {
		t.Fatalf("released alert claim could not be retried: claim=%t err=%v", claim, err)
	}
	for _, key := range server.Keys() {
		if strings.Contains(key, "private-company-id") {
			t.Fatalf("Redis overage key contains raw tenant identity: %q", key)
		}
	}
}

func TestProductRuntimeSendsOneWarningAndOneExceededNotification(t *testing.T) {
	ledger := &memoryUsageLedger{}
	audit := &memoryAuditLog{}
	webhooks := &testWebhookSender{}
	policies, err := NewTierPolicyCatalog([]TierPolicy{{
		Tier: PlanPro, MonthlyRequestLimit: 4, WarningPercent: 50, AlertThreshold: 60,
		Features: FeatureEntitlements{WebhookAlerts: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewProductRuntime(ProductServices{
		TenantResolver: func(*schemas.BifrostContext) string { return "trusted-company" },
		UsageLedger:    ledger,
		PlanResolver:   testPlanResolver(func(context.Context, string) (PlanTier, error) { return PlanPro, nil }),
		Policies:       policies,
		AuditLog:       audit,
		Webhooks:       webhooks,
		QueueSize:      8,
		Workers:        1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	for i, tokens := range []int64{1, 1, 1, 1, 1} {
		if !runtime.SubmitUsage(UsageEvent{
			EventID: "overage-event-000" + strconv.Itoa(i+1), TenantID: "trusted-company",
			OccurredAt: now, PromptTokens: tokens,
		}) {
			t.Fatalf("usage event %d was not queued", i)
		}
	}
	waitFor(t, func() bool {
		audit.mu.Lock()
		defer audit.mu.Unlock()
		webhooks.mu.Lock()
		defer webhooks.mu.Unlock()
		return len(audit.events) == 2 && len(webhooks.records) == 2
	})

	audit.mu.Lock()
	defer audit.mu.Unlock()
	webhooks.mu.Lock()
	defer webhooks.mu.Unlock()
	if audit.events[0].Kind != "usage_limit_warning" || audit.events[1].Kind != "usage_limit_exceeded" {
		t.Fatalf("expected one warning and one exceeded audit event: %+v", audit.events)
	}
	if !audit.events[0].Alert || !audit.events[1].Alert || len(webhooks.records) != 2 {
		t.Fatalf("expected advisory notifications for both stages: audits=%+v webhooks=%d", audit.events, len(webhooks.records))
	}
	for _, body := range []AuditEvent{audit.events[0], audit.events[1]} {
		if body.PolicyOutcome == "" {
			t.Fatalf("overage notification omitted its advisory outcome: %+v", body)
		}
	}
}
