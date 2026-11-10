// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"strings"
	"testing"
	"time"
)

func TestConfigFromLookupAppliesValidatedEnvironmentOverrides(t *testing.T) {
	values := map[string]string{
		"FRAUDGUARD_ALERT_THRESHOLD":      "75",
		"FRAUDGUARD_WINDOW":               "5m",
		"FRAUDGUARD_ENABLE_CONTEXT_GUARD": "true",
		"FRAUDGUARD_MAX_CONTEXT_MESSAGES": "64",
		"UNRELATED_SECRET":                "must-not-be-read",
	}
	config, err := configFromLookup(Config{AlertThreshold: 55}, func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.AlertThreshold != 75 || config.Window != 5*time.Minute ||
		!config.EnableContextGuard || config.MaxContextMessages != 64 {
		t.Fatalf("environment overrides were not applied: %+v", config)
	}
}

func TestConfigFromLookupRejectsInvalidBoundsAndDoesNotEchoValues(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]string
		base  Config
	}{
		{name: "threshold", input: map[string]string{"FRAUDGUARD_ALERT_THRESHOLD": "101"}},
		{name: "bad integer", input: map[string]string{"FRAUDGUARD_WARMUP_REQUESTS": "oops"}},
		{name: "bad bool", input: map[string]string{"FRAUDGUARD_ENABLE_CONTEXT_GUARD": "yes"}},
		{name: "duration lower bound", input: map[string]string{"FRAUDGUARD_WINDOW": "1ns"}},
		{name: "base config", base: Config{MaxProfiles: -1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := configFromLookup(test.base, func(key string) (string, bool) {
				value, ok := test.input[key]
				return value, ok
			})
			if err == nil {
				t.Fatal("expected invalid configuration to be rejected")
			}
			if strings.Contains(err.Error(), "oops") || strings.Contains(err.Error(), "101") {
				t.Fatalf("validation error echoed the raw environment value: %v", err)
			}
		})
	}
}

func TestValidateConfigAcceptsDefaultsAndRejectsInvalidPaymentThreshold(t *testing.T) {
	if err := ValidateConfig(Config{}); err != nil {
		t.Fatalf("zero values should select defaults: %v", err)
	}
	if err := ValidateConfig(Config{MaxPaymentsPerWindow: -1}); err == nil {
		t.Fatal("negative payment velocity threshold was accepted")
	}
}
