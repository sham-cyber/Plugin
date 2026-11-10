// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
)

type PlanTier string

const (
	PlanFree       PlanTier = "free"
	PlanPro        PlanTier = "pro"
	PlanEnterprise PlanTier = "enterprise"
)

func (tier PlanTier) Valid() bool {
	return tier == PlanFree || tier == PlanPro || tier == PlanEnterprise
}

// FeatureEntitlements describes product capabilities, not request permission.
// A false value never blocks or rewrites an inference request.
type FeatureEntitlements struct {
	PromptSignals    bool `json:"prompt_signals"`
	PaymentSignals   bool `json:"payment_signals"`
	KeyStatusSignals bool `json:"key_status_signals"`
	CrossKeySignals  bool `json:"cross_key_signals"`
	AIReview         bool `json:"ai_review"`
	UsageExport      bool `json:"usage_export"`
	WebhookAlerts    bool `json:"webhook_alerts"`
}

// TierPolicy contains operator-selected commercial limits and entitlements.
// Limits of zero mean "not configured"; no prices or quotas are invented.
type TierPolicy struct {
	Tier                 PlanTier            `json:"tier"`
	MonthlyRequestLimit  int64               `json:"monthly_request_limit,omitempty"`
	MonthlyTokenLimit    int64               `json:"monthly_token_limit,omitempty"`
	MaxPaymentsPerWindow int                 `json:"max_payments_per_window,omitempty"`
	AlertThreshold       int                 `json:"alert_threshold"`
	WarningPercent       int                 `json:"warning_percent,omitempty"`
	Features             FeatureEntitlements `json:"features"`
}

func (policy TierPolicy) Validate() error {
	if !policy.Tier.Valid() {
		return fmt.Errorf("fraudguard: invalid plan tier %q", policy.Tier)
	}
	if policy.MonthlyRequestLimit < 0 || policy.MonthlyTokenLimit < 0 || policy.MaxPaymentsPerWindow < 0 {
		return errors.New("fraudguard: plan limits cannot be negative")
	}
	if policy.MaxPaymentsPerWindow > 10_000 {
		return errors.New("fraudguard: per-window payment limit cannot exceed 10000")
	}
	if policy.AlertThreshold < 0 || policy.AlertThreshold > 100 {
		return errors.New("fraudguard: plan alert threshold must be between 0 and 100")
	}
	if policy.WarningPercent == 0 {
		return nil
	}
	if policy.WarningPercent < 1 || policy.WarningPercent > 100 {
		return errors.New("fraudguard: warning percent must be between 1 and 100")
	}
	return nil
}

func (policy TierPolicy) warningPercent() int {
	if policy.WarningPercent == 0 {
		return 80
	}
	return policy.WarningPercent
}

// TierPolicyCatalog is an immutable policy table supplied by the host's
// trusted billing/configuration layer.
type TierPolicyCatalog struct {
	policies map[PlanTier]TierPolicy
}

func NewTierPolicyCatalog(policies []TierPolicy) (*TierPolicyCatalog, error) {
	catalog := &TierPolicyCatalog{policies: make(map[PlanTier]TierPolicy, len(policies))}
	for _, policy := range policies {
		if err := policy.Validate(); err != nil {
			return nil, err
		}
		if _, exists := catalog.policies[policy.Tier]; exists {
			return nil, fmt.Errorf("fraudguard: duplicate policy for tier %q", policy.Tier)
		}
		catalog.policies[policy.Tier] = policy
	}
	if len(catalog.policies) == 0 {
		return nil, errors.New("fraudguard: at least one tier policy is required")
	}
	return catalog, nil
}

func (catalog *TierPolicyCatalog) Resolve(tier PlanTier) (TierPolicy, error) {
	if catalog == nil {
		return TierPolicy{}, errors.New("fraudguard: tier policy catalog is not configured")
	}
	policy, ok := catalog.policies[tier]
	if !ok {
		return TierPolicy{}, fmt.Errorf("fraudguard: no policy configured for tier %q", tier)
	}
	return policy, nil
}

func (catalog *TierPolicyCatalog) Tiers() []PlanTier {
	if catalog == nil {
		return nil
	}
	tiers := make([]PlanTier, 0, len(catalog.policies))
	for tier := range catalog.policies {
		tiers = append(tiers, tier)
	}
	sort.Slice(tiers, func(i, j int) bool { return tiers[i] < tiers[j] })
	return tiers
}

// PolicyEvaluation is informational only. Exceeded=true records an overage;
// it does not reject, pause, or alter requests.
type PolicyEvaluation struct {
	Tier                    PlanTier            `json:"tier"`
	EffectiveAlertThreshold int                 `json:"effective_alert_threshold"`
	MaxPaymentsPerWindow    int                 `json:"max_payments_per_window,omitempty"`
	Features                FeatureEntitlements `json:"features"`
	RequestWarning          bool                `json:"request_warning"`
	TokenWarning            bool                `json:"token_warning"`
	RequestLimitExceeded    bool                `json:"request_limit_exceeded"`
	TokenLimitExceeded      bool                `json:"token_limit_exceeded"`
	Exceeded                bool                `json:"exceeded"`
	Outcome                 string              `json:"outcome"`
}

func (policy TierPolicy) EvaluateUsage(usage UsageTotals) PolicyEvaluation {
	result := PolicyEvaluation{
		Tier:                    policy.Tier,
		EffectiveAlertThreshold: policy.AlertThreshold,
		MaxPaymentsPerWindow:    policy.MaxPaymentsPerWindow,
		Features:                policy.Features,
		Outcome:                 "within_limit",
	}
	tokens := saturatingAddNonnegative(usage.PromptTokens, usage.CompletionTokens)
	if policy.MonthlyRequestLimit > 0 {
		result.RequestWarning = reachedPercent(usage.Requests, policy.MonthlyRequestLimit, policy.warningPercent())
		result.RequestLimitExceeded = usage.Requests > policy.MonthlyRequestLimit
	}
	if policy.MonthlyTokenLimit > 0 {
		result.TokenWarning = reachedPercent(tokens, policy.MonthlyTokenLimit, policy.warningPercent())
		result.TokenLimitExceeded = tokens > policy.MonthlyTokenLimit
	}
	result.Exceeded = result.RequestLimitExceeded || result.TokenLimitExceeded
	if result.Exceeded {
		result.Outcome = "limit_exceeded_observe_only"
	} else if result.RequestWarning || result.TokenWarning {
		result.Outcome = "limit_warning_observe_only"
	}
	return result
}

func (policy TierPolicy) AlertForScore(score int) bool {
	if score < 0 {
		return false
	}
	return score >= policy.AlertThreshold
}

func reachedPercent(value, limit int64, percent int) bool {
	if limit <= 0 || value < 0 {
		return false
	}
	p := int64(percent)
	whole := (limit / 100) * p
	remainder := ((limit%100)*p + 99) / 100
	return value >= whole+remainder
}

func saturatingAddNonnegative(a, b int64) int64 {
	if a < 0 || b < 0 {
		return 0
	}
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

type TenantTierResolver interface {
	ResolveTenantTier(context.Context, string) (PlanTier, error)
}
