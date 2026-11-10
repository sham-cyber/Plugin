// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrUsageExportNotEntitled = errors.New("fraudguard: usage export is not enabled for this tenant's plan")

// UsageExport is a versioned, amount-free usage report. It is suitable for a
// host billing system to ingest, but it is not an invoice or payment request.
type UsageExport struct {
	SchemaVersion string            `json:"schema_version"`
	GeneratedAt   time.Time         `json:"generated_at"`
	Period        string            `json:"period"`
	TenantRef     string            `json:"tenant_ref"`
	Usage         UsageTotals       `json:"usage"`
	Plan          *PolicyEvaluation `json:"plan,omitempty"`
}

func BuildUsageExport(
	ctx context.Context,
	ledger UsageLedger,
	tenantID string,
	month string,
	plans TenantTierResolver,
	catalog *TierPolicyCatalog,
) (UsageExport, error) {
	if ledger == nil {
		return UsageExport{}, errors.New("fraudguard: usage ledger is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return UsageExport{}, ErrInvalidTenantID
	}
	if !validUsageMonth(month) {
		return UsageExport{}, errors.New("fraudguard: month must use YYYY-MM format")
	}
	var policy *TierPolicy
	if plans != nil {
		tier, resolveErr := plans.ResolveTenantTier(ctx, tenantID)
		if resolveErr != nil {
			return UsageExport{}, resolveErr
		}
		if catalog == nil {
			return UsageExport{}, errors.New("fraudguard: a policy catalog is required when tenant plans are enabled")
		}
		resolved, policyErr := catalog.Resolve(tier)
		if policyErr != nil {
			return UsageExport{}, policyErr
		}
		if !resolved.Features.UsageExport {
			return UsageExport{}, ErrUsageExportNotEntitled
		}
		policy = &resolved
	}
	usage, err := ledger.UsageForMonth(ctx, tenantID, month)
	if err != nil {
		return UsageExport{}, err
	}
	result := UsageExport{
		SchemaVersion: "fraudguard.usage.v2",
		GeneratedAt:   time.Now().UTC(),
		Period:        month,
		TenantRef:     usage.TenantRef,
		Usage:         usage,
	}
	if policy != nil {
		evaluation := policy.EvaluateUsage(usage)
		result.Plan = &evaluation
	}
	return result, nil
}

func ExportUsageJSON(
	ctx context.Context,
	ledger UsageLedger,
	tenantID string,
	month string,
	plans TenantTierResolver,
	catalog *TierPolicyCatalog,
) ([]byte, error) {
	report, err := BuildUsageExport(ctx, ledger, tenantID, month, plans, catalog)
	if err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return payload, nil
}
