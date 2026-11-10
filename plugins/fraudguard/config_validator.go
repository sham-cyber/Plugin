// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ConfigFromEnvironment applies documented FRAUDGUARD_* startup overrides to a
// caller-supplied base config and validates the result. It does not read secrets.
func ConfigFromEnvironment(base Config) (Config, error) {
	return configFromLookup(base, os.LookupEnv)
}

// NewWithProductServicesFromEnvironment applies the same startup overrides
// before constructing a plugin with host-provided product services.
func NewWithProductServicesFromEnvironment(base Config, services ProductServices) (*Plugin, error) {
	config, err := ConfigFromEnvironment(base)
	if err != nil {
		return nil, err
	}
	return NewWithProductServices(config, services)
}

// ValidateConfig rejects out-of-range startup settings. Zero means "use the
// documented default"; this does not add request-rate limiting.
func ValidateConfig(config Config) error {
	if config.EnableCustomPromptRules && config.CustomPromptRules == nil {
		return fmt.Errorf("fraudguard: custom prompt rules enabled without a rule set")
	}
	integerBounds := []struct {
		name       string
		value, min int
		max        int
	}{
		{"alert_threshold", config.AlertThreshold, 1, 100},
		{"warmup_requests", config.WarmupRequests, 1, 10_000},
		{"max_profiles", config.MaxProfiles, 1, 1_000_000},
		{"max_events_per_profile", config.MaxEventsPerProfile, 1, 10_000},
		{"max_distinct_keys_per_identity", config.MaxDistinctKeysPerIdentity, 1, 100},
		{"max_payment_subjects", config.MaxPaymentSubjects, 1, 1_000_000},
		{"max_payment_events_per_subject", config.MaxPaymentEventsPerSubject, 1, 10_000},
		{"max_payments_per_window", config.MaxPaymentsPerWindow, 1, 100_000},
		{"payment_location_warmup_events", config.PaymentLocationWarmupEvents, 1, 10_000},
		{"max_prompt_bytes", config.MaxPromptBytes, 1, 4 * 1024 * 1024},
		{"max_prompt_scan_bytes", config.MaxPromptScanBytes, 1, 1024 * 1024},
		{"max_context_messages", config.MaxContextMessages, 1, maxContextMessages},
		{"max_context_scan_bytes", config.MaxContextScanBytes, 1, maxContextScanBytes},
		{"finding_callback_queue_size", config.FindingCallbackQueueSize, 1, 4096},
		{"ai_analysis_min_score", config.AIAnalysisMinScore, 1, 100},
	}
	for _, bound := range integerBounds {
		if err := validateOptionalInt(bound.name, bound.value, bound.min, bound.max); err != nil {
			return err
		}
	}

	durationBounds := []struct {
		name       string
		value, min time.Duration
		max        time.Duration
	}{
		{"window", config.Window, time.Second, 24 * time.Hour},
		{"profile_ttl", config.ProfileTTL, time.Minute, 30 * 24 * time.Hour},
		{"correlation_window", config.CorrelationWindow, time.Second, 24 * time.Hour},
		{"payment_window", config.PaymentWindow, time.Second, 24 * time.Hour},
		{"payment_profile_ttl", config.PaymentProfileTTL, time.Minute, 30 * 24 * time.Hour},
	}
	for _, bound := range durationBounds {
		if err := validateOptionalDuration(bound.name, bound.value, bound.min, bound.max); err != nil {
			return err
		}
	}

	if len(config.ExemptVirtualKeyIDs) > 10_000 {
		return fmt.Errorf("fraudguard: exempt_virtual_key_ids cannot exceed 10000 entries")
	}
	for _, id := range config.ExemptVirtualKeyIDs {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 256 {
			return fmt.Errorf("fraudguard: exempt_virtual_key_ids entries must be 1 to 256 bytes")
		}
	}
	return nil
}

func configFromLookup(base Config, lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		return Config{}, fmt.Errorf("fraudguard: environment lookup is required")
	}
	intOverrides := []struct {
		name string
		min  int
		max  int
		set  func(int)
	}{
		{"FRAUDGUARD_ALERT_THRESHOLD", 1, 100, func(v int) { base.AlertThreshold = v }},
		{"FRAUDGUARD_WARMUP_REQUESTS", 1, 10_000, func(v int) { base.WarmupRequests = v }},
		{"FRAUDGUARD_MAX_PROFILES", 1, 1_000_000, func(v int) { base.MaxProfiles = v }},
		{"FRAUDGUARD_MAX_EVENTS_PER_PROFILE", 1, 10_000, func(v int) { base.MaxEventsPerProfile = v }},
		{"FRAUDGUARD_MAX_DISTINCT_KEYS_PER_IDENTITY", 1, 100, func(v int) { base.MaxDistinctKeysPerIdentity = v }},
		{"FRAUDGUARD_MAX_PAYMENT_SUBJECTS", 1, 1_000_000, func(v int) { base.MaxPaymentSubjects = v }},
		{"FRAUDGUARD_MAX_PAYMENT_EVENTS_PER_SUBJECT", 1, 10_000, func(v int) { base.MaxPaymentEventsPerSubject = v }},
		{"FRAUDGUARD_MAX_PAYMENTS_PER_WINDOW", 1, 100_000, func(v int) { base.MaxPaymentsPerWindow = v }},
		{"FRAUDGUARD_PAYMENT_LOCATION_WARMUP_EVENTS", 1, 10_000, func(v int) { base.PaymentLocationWarmupEvents = v }},
		{"FRAUDGUARD_MAX_PROMPT_BYTES", 1, 4 * 1024 * 1024, func(v int) { base.MaxPromptBytes = v }},
		{"FRAUDGUARD_MAX_PROMPT_SCAN_BYTES", 1, 1024 * 1024, func(v int) { base.MaxPromptScanBytes = v }},
		{"FRAUDGUARD_MAX_CONTEXT_MESSAGES", 1, maxContextMessages, func(v int) { base.MaxContextMessages = v }},
		{"FRAUDGUARD_MAX_CONTEXT_SCAN_BYTES", 1, maxContextScanBytes, func(v int) { base.MaxContextScanBytes = v }},
		{"FRAUDGUARD_FINDING_CALLBACK_QUEUE_SIZE", 1, 4096, func(v int) { base.FindingCallbackQueueSize = v }},
		{"FRAUDGUARD_AI_ANALYSIS_MIN_SCORE", 1, 100, func(v int) { base.AIAnalysisMinScore = v }},
	}
	for _, override := range intOverrides {
		value, ok := lookup(override.name)
		if !ok {
			continue
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("fraudguard: %s must be an integer", override.name)
		}
		if parsed < override.min || parsed > override.max {
			return Config{}, fmt.Errorf("fraudguard: %s must be between %d and %d", override.name, override.min, override.max)
		}
		override.set(parsed)
	}

	durationOverrides := []struct {
		name string
		min  time.Duration
		max  time.Duration
		set  func(time.Duration)
	}{
		{"FRAUDGUARD_WINDOW", time.Second, 24 * time.Hour, func(v time.Duration) { base.Window = v }},
		{"FRAUDGUARD_PROFILE_TTL", time.Minute, 30 * 24 * time.Hour, func(v time.Duration) { base.ProfileTTL = v }},
		{"FRAUDGUARD_CORRELATION_WINDOW", time.Second, 24 * time.Hour, func(v time.Duration) { base.CorrelationWindow = v }},
		{"FRAUDGUARD_PAYMENT_WINDOW", time.Second, 24 * time.Hour, func(v time.Duration) { base.PaymentWindow = v }},
		{"FRAUDGUARD_PAYMENT_PROFILE_TTL", time.Minute, 30 * 24 * time.Hour, func(v time.Duration) { base.PaymentProfileTTL = v }},
	}
	for _, override := range durationOverrides {
		value, ok := lookup(override.name)
		if !ok {
			continue
		}
		parsed, err := time.ParseDuration(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("fraudguard: %s must be a duration", override.name)
		}
		if err := validateOptionalDuration(override.name, parsed, override.min, override.max); err != nil {
			return Config{}, err
		}
		override.set(parsed)
	}

	boolOverrides := []struct {
		name string
		set  func(bool)
	}{
		{"FRAUDGUARD_ENABLE_CROSS_KEY_CORRELATION", func(v bool) { base.EnableCrossKeyCorrelation = v }},
		{"FRAUDGUARD_ENABLE_PAYMENT_VELOCITY", func(v bool) { base.EnablePaymentVelocity = v }},
		{"FRAUDGUARD_ENABLE_PAYMENT_LOCATION", func(v bool) { base.EnablePaymentLocation = v }},
		{"FRAUDGUARD_ENABLE_EXPIRED_KEY_SIGNAL", func(v bool) { base.EnableExpiredKeySignal = v }},
		{"FRAUDGUARD_ENABLE_REVOKED_KEY_SIGNAL", func(v bool) { base.EnableRevokedKeySignal = v }},
		{"FRAUDGUARD_ENABLE_EXPOSED_KEY_SIGNAL", func(v bool) { base.EnableExposedKeySignal = v }},
		{"FRAUDGUARD_ENABLE_PROMPT_OVERRIDE_SIGNALS", func(v bool) { base.EnablePromptOverrideSignals = v }},
		{"FRAUDGUARD_ENABLE_PROMPT_LENGTH_SIGNAL", func(v bool) { base.EnablePromptLengthSignal = v }},
		{"FRAUDGUARD_ENABLE_CONTEXT_GUARD", func(v bool) { base.EnableContextGuard = v }},
	}
	for _, override := range boolOverrides {
		value, ok := lookup(override.name)
		if !ok {
			continue
		}
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("fraudguard: %s must be true or false", override.name)
		}
		override.set(parsed)
	}

	if err := ValidateConfig(base); err != nil {
		return Config{}, err
	}
	return base, nil
}

func validateOptionalInt(name string, value, min, max int) error {
	if value == 0 {
		return nil
	}
	if value < min || value > max {
		return fmt.Errorf("fraudguard: %s must be between %d and %d", name, min, max)
	}
	return nil
}

func validateOptionalDuration(name string, value, min, max time.Duration) error {
	if value == 0 {
		return nil
	}
	if value < min || value > max {
		return fmt.Errorf("fraudguard: %s must be between %s and %s", name, min, max)
	}
	return nil
}
