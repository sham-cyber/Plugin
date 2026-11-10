// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildMetadataAcceptsBoundedReleaseValuesAndSanitizesOthers(t *testing.T) {
	got := buildMetadataFor("v1.2.3-rc.1", "0123456789ab", "2026-11-10T10:30:00+05:30")
	if got.Version != "v1.2.3-rc.1" || got.Commit != "0123456789ab" ||
		got.BuildTime != "2026-11-10T05:00:00Z" {
		t.Fatalf("unexpected normalized build metadata: %+v", got)
	}

	got = buildMetadataFor("not-semver-secret", "commit-secret", "secret-time")
	if got.Version != "dev" || got.Commit != "unknown" || got.BuildTime != "unknown" {
		t.Fatalf("invalid build metadata was not replaced with safe values: %+v", got)
	}
}

func TestSecuritySanityReportIsFixedAndDoesNotExposeValues(t *testing.T) {
	privateInternalID := "PRIVATE_INTERNAL_ID_SENTINEL"
	report := RunSecurityChecks(Config{
		ExemptVirtualKeyIDs:         []string{privateInternalID},
		EnablePromptOverrideSignals: true,
	})
	if report.Status != "configuration_valid" || report.SchemaVersion != "fraudguard.security.v1" {
		t.Fatalf("unexpected security report: %+v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{privateInternalID, "secret-value", "Authorization"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("security report leaked %q: %s", forbidden, encoded)
		}
	}
	if !securityCheckHas(report, "secret_values", "not_inspected", "host_managed") ||
		!securityCheckHas(report, "request_handling", "passed", "observe_only") ||
		!securityCheckHas(report, "prompt_inspection", "opt_in_bounded", "transient_user_text_only") ||
		!securityCheckHas(report, "debug_token_auth", "not_present", "no_debug_token_feature") {
		t.Fatalf("security report missed fixed checks: %+v", report.Checks)
	}
	invalid := RunSecurityChecks(Config{AlertThreshold: 101})
	if invalid.Status != "configuration_invalid" ||
		!securityCheckHas(invalid, "configuration_bounds", "failed", "invalid_configuration") {
		t.Fatalf("invalid configuration did not produce a generic failure report: %+v", invalid)
	}
}

func TestStartupSecurityGuardRejectsInvalidBoundsWithoutEchoingInput(t *testing.T) {
	err := ValidateStartupSecurity(Config{AlertThreshold: 101})
	if err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatalf("invalid configuration was not safely rejected: %v", err)
	}
	if err := ValidateStartupSecurity(Config{}); err != nil {
		t.Fatalf("safe default config failed startup check: %v", err)
	}
}

func TestHealthResponseIncludesSafeBuildAndSecurityMetadata(t *testing.T) {
	plugin, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer plugin.Cleanup()

	status := plugin.CheckHealth(context.Background(), HealthOptions{})
	if status.Build.Version == "" || status.Build.Commit == "" ||
		status.Security == nil || status.Security.Status != "configuration_valid" {
		t.Fatalf("health response is missing safe build/security information: %+v", status)
	}
}

func securityCheckHas(report SecurityCheckReport, name, status, code string) bool {
	for _, check := range report.Checks {
		if check.Name == name && check.Status == status && check.Code == code {
			return true
		}
	}
	return false
}
