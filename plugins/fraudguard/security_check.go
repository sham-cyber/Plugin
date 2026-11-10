// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import "errors"

// SecurityCheckResult reports a fixed check name, status, and code. It never
// includes environment values, secret contents, tenant identifiers, or errors.
type SecurityCheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Code   string `json:"code"`
}

// SecurityCheckReport is a safe, aggregate-only startup sanity summary.
type SecurityCheckReport struct {
	SchemaVersion string                `json:"schema_version"`
	Status        string                `json:"status"`
	Checks        []SecurityCheckResult `json:"checks"`
}

// RunSecurityChecks validates detector bounds and reports fixed architectural
// properties. It does not enumerate environment variables, inspect secret
// values, verify JWTs, or scan dependencies.
func RunSecurityChecks(config Config) SecurityCheckReport {
	report := SecurityCheckReport{
		SchemaVersion: "fraudguard.security.v1",
		Status:        "configuration_valid",
		Checks:        make([]SecurityCheckResult, 0, 7),
	}

	configStatus := "passed"
	configCode := "configuration_within_bounds"
	if ValidateConfig(config) != nil {
		configStatus = "failed"
		configCode = "invalid_configuration"
		report.Status = "configuration_invalid"
	}
	report.Checks = append(report.Checks, SecurityCheckResult{
		Name: "configuration_bounds", Status: configStatus, Code: configCode,
	})

	report.Checks = append(report.Checks,
		SecurityCheckResult{Name: "request_handling", Status: "passed", Code: "observe_only"},
		SecurityCheckResult{Name: "identity_source", Status: "passed", Code: "bifrost_resolved_context"},
	)

	promptChecksEnabled := config.EnablePromptOverrideSignals ||
		config.EnablePromptLengthSignal ||
		config.EnableCustomPromptRules ||
		config.EnableContextGuard
	promptStatus := "disabled"
	promptCode := "prompt_inspection_off"
	if promptChecksEnabled {
		promptStatus = "opt_in_bounded"
		promptCode = "transient_user_text_only"
	}
	report.Checks = append(report.Checks, SecurityCheckResult{
		Name: "prompt_inspection", Status: promptStatus, Code: promptCode,
	})

	report.Checks = append(report.Checks,
		SecurityCheckResult{Name: "secret_values", Status: "not_inspected", Code: "host_managed"},
		SecurityCheckResult{Name: "jwt_verification", Status: "not_applicable", Code: "host_authentication_required"},
		SecurityCheckResult{Name: "debug_token_auth", Status: "not_present", Code: "no_debug_token_feature"},
	)
	return report
}

// ValidateStartupSecurity is the bounded configuration guard used by startup
// configuration loaders. Secret material remains under host-owned handling.
func ValidateStartupSecurity(config Config) error {
	report := RunSecurityChecks(config)
	if report.Status == "configuration_valid" {
		return nil
	}
	if err := ValidateConfig(config); err != nil {
		return err
	}
	return errors.New("fraudguard: startup security checks failed")
}
