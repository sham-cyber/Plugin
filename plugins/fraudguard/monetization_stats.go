// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"errors"
	"time"
)

// MonetizationStats is an aggregate, observe-only view for a host's private
// financial dashboard. It deliberately excludes prices, invoices, profit,
// blocked-request counts, and tenant identifiers.
type MonetizationStats struct {
	SchemaVersion              string             `json:"schema_version"`
	GeneratedAt                time.Time          `json:"generated_at"`
	RequestHandling            string             `json:"request_handling"`
	FraudGuardBillingConnected bool               `json:"fraudguard_billing_connected"`
	FinancialActualsAvailable  bool               `json:"financial_actuals_available"`
	TierDistributionScope      string             `json:"tier_distribution_scope"`
	TierDistribution           map[PlanTier]int64 `json:"tier_distribution"`
	Product                    ProductMetrics     `json:"product"`
	Budget                     BudgetMetrics      `json:"budget"`
}

func buildMonetizationStats(plugin *Plugin) MonetizationStats {
	distribution := map[PlanTier]int64{}
	scope := "not_configured"
	product := ProductMetrics{}
	budget := BudgetMetrics{}
	if plugin != nil {
		product = plugin.ProductMetrics()
		budget = plugin.BudgetMetrics()
		if plugin.product != nil {
			distribution = plugin.product.CachedTierDistribution()
			scope = "current_process_policy_cache_only"
		}
	}
	return MonetizationStats{
		SchemaVersion:              "fraudguard.monetization.v1",
		GeneratedAt:                time.Now().UTC(),
		RequestHandling:            "observe_only_pass_through",
		FraudGuardBillingConnected: false,
		FinancialActualsAvailable:  false,
		TierDistributionScope:      scope,
		TierDistribution:           distribution,
		Product:                    product,
		Budget:                     budget,
	}
}

func validateTierDistribution(distribution map[PlanTier]int64) (map[PlanTier]int64, error) {
	if len(distribution) > 3 {
		return nil, errors.New("fraudguard: tier distribution contains too many plan types")
	}
	result := make(map[PlanTier]int64, len(distribution))
	for tier, count := range distribution {
		if !tier.Valid() || count < 0 {
			return nil, errors.New("fraudguard: tier distribution contains invalid data")
		}
		if count > 0 {
			result[tier] = count
		}
	}
	return result, nil
}
