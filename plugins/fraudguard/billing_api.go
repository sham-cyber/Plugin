// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const BillingUsageExportPath = "/admin/fraudguard/usage-export"

// BillingExportAuthorizer must authenticate the billing service or operator.
// The host owns its credentials and authorization rules.
type BillingExportAuthorizer func(*http.Request) bool

// BillingExportTenantResolver must return the tenant resolved from trusted,
// authenticated host context; it must not trust caller-supplied tenant headers.
type BillingExportTenantResolver func(*http.Request) (string, error)

type BillingExportHandlerOptions struct {
	Authorize     BillingExportAuthorizer
	ResolveTenant BillingExportTenantResolver
	Ledger        UsageLedger
	Plans         TenantTierResolver
	Catalog       *TierPolicyCatalog
}

// NewBillingExportHandler serves a pull-based, amount-free usage report.
// It does not connect to an external billing provider or create invoices.
func NewBillingExportHandler(options BillingExportHandlerOptions) (http.Handler, error) {
	switch {
	case options.Authorize == nil:
		return nil, errors.New("fraudguard: billing export authorization callback is required")
	case options.ResolveTenant == nil:
		return nil, errors.New("fraudguard: trusted billing tenant resolver is required")
	case options.Ledger == nil:
		return nil, errors.New("fraudguard: billing usage ledger is required")
	case (options.Plans == nil) != (options.Catalog == nil):
		return nil, errors.New("fraudguard: plan resolver and policy catalog must be configured together")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path != BillingUsageExportPath {
			writeBillingExportError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeBillingExportError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		if !options.Authorize(r) {
			writeBillingExportError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
			return
		}

		query := r.URL.Query()
		months := query["month"]
		if len(query) != 1 || len(months) != 1 || !validUsageMonth(months[0]) {
			writeBillingExportError(w, http.StatusBadRequest, "invalid_period", "provide exactly one month in YYYY-MM format")
			return
		}

		tenantID, err := options.ResolveTenant(r)
		if err != nil || strings.TrimSpace(tenantID) == "" {
			writeBillingExportError(w, http.StatusForbidden, "tenant_unavailable", "tenant access is unavailable")
			return
		}

		report, err := BuildUsageExport(r.Context(), options.Ledger, tenantID, months[0], options.Plans, options.Catalog)
		switch {
		case errors.Is(err, ErrUsageExportNotEntitled):
			writeBillingExportError(w, http.StatusForbidden, "export_not_entitled", "usage export is not enabled for this plan")
			return
		case err != nil:
			writeBillingExportError(w, http.StatusServiceUnavailable, "export_unavailable", "usage export is temporarily unavailable")
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(report); err != nil {
			// The response may already be partially written; do not log request
			// data or attempt to expose internal storage errors to the caller.
			return
		}
	}), nil
}

func writeBillingExportError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{
		Error: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{Code: code, Message: message},
	})
}
