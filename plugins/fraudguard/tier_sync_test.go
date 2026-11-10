// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestTierSyncRefreshesCachedPolicyAfterTrustedPlanUpdate(t *testing.T) {
	currentTier := PlanFree
	resolver := testPlanResolver(func(context.Context, string) (PlanTier, error) {
		return currentTier, nil
	})
	catalog, err := NewTierPolicyCatalog([]TierPolicy{
		{Tier: PlanFree, AlertThreshold: 70},
		{Tier: PlanEnterprise, AlertThreshold: 40, Features: FeatureEntitlements{UsageExport: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewProductRuntime(ProductServices{
		TenantResolver: func(*schemas.BifrostContext) string { return "trusted-tenant" },
		UsageLedger:    &memoryUsageLedger{},
		PlanResolver:   resolver,
		Policies:       catalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	before, found, err := runtime.ResolvePolicy(context.Background(), "trusted-tenant")
	if err != nil || !found || before.Tier != PlanFree {
		t.Fatalf("initial policy was not cached: policy=%+v found=%t err=%v", before, found, err)
	}
	currentTier = PlanEnterprise
	manager, err := NewTierSyncManager(runtime)
	if err != nil {
		t.Fatal(err)
	}
	after, err := manager.RefreshAfterTrustedUpdate(context.Background(), "trusted-tenant")
	if err != nil || after.Tier != PlanEnterprise || after.AlertThreshold != 40 {
		t.Fatalf("verified tier update was not applied immediately: policy=%+v err=%v", after, err)
	}
	cached, found := runtime.CachedPolicy("trusted-tenant")
	if !found || cached.Tier != PlanEnterprise {
		t.Fatalf("refreshed policy was not visible to inference hooks: policy=%+v found=%t", cached, found)
	}
	distribution := runtime.CachedTierDistribution()
	if distribution[PlanEnterprise] != 1 || distribution[PlanFree] != 0 {
		t.Fatalf("cached plan distribution was not updated: %+v", distribution)
	}
}

func TestTierSyncRejectsMissingTrustedTenant(t *testing.T) {
	catalog, err := NewTierPolicyCatalog([]TierPolicy{{Tier: PlanFree, AlertThreshold: 60}})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewProductRuntime(ProductServices{
		TenantResolver: func(*schemas.BifrostContext) string { return "trusted-tenant" },
		UsageLedger:    &memoryUsageLedger{},
		PlanResolver:   testPlanResolver(func(context.Context, string) (PlanTier, error) { return PlanFree, nil }),
		Policies:       catalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	manager, err := NewTierSyncManager(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RefreshAfterTrustedUpdate(context.Background(), " "); err == nil {
		t.Fatal("blank tenant identity was accepted")
	}
}
