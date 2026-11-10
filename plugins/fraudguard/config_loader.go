// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const maxRuntimeConfigBytes = 1 << 20

// RuntimeConfigDocument is a full, versioned snapshot of the settings that are
// safe to change while the engine is running. It contains no blocking rules.
// Durations use Go duration strings such as "15m" and "2h".
type RuntimeConfigDocument struct {
	Version                     int      `json:"version"`
	AlertThreshold              int      `json:"alert_threshold"`
	WarmupRequests              int      `json:"warmup_requests"`
	Window                      string   `json:"window"`
	ProfileTTL                  string   `json:"profile_ttl"`
	MaxProfiles                 int      `json:"max_profiles"`
	MaxEventsPerProfile         int      `json:"max_events_per_profile"`
	EnableCrossKeyCorrelation   bool     `json:"enable_cross_key_correlation"`
	MaxDistinctKeysPerIdentity  int      `json:"max_distinct_keys_per_identity"`
	CorrelationWindow           string   `json:"correlation_window"`
	EnablePaymentVelocity       bool     `json:"enable_payment_velocity"`
	EnablePaymentLocation       bool     `json:"enable_payment_location"`
	PaymentWindow               string   `json:"payment_window"`
	PaymentProfileTTL           string   `json:"payment_profile_ttl"`
	MaxPaymentSubjects          int      `json:"max_payment_subjects"`
	MaxPaymentEventsPerSubject  int      `json:"max_payment_events_per_subject"`
	MaxPaymentsPerWindow        int      `json:"max_payments_per_window"`
	PaymentLocationWarmupEvents int      `json:"payment_location_warmup_events"`
	EnableExpiredKeySignal      bool     `json:"enable_expired_key_signal"`
	EnableRevokedKeySignal      bool     `json:"enable_revoked_key_signal"`
	EnableExposedKeySignal      bool     `json:"enable_exposed_key_signal"`
	EnablePromptOverrideSignals bool     `json:"enable_prompt_override_signals"`
	EnablePromptLengthSignal    bool     `json:"enable_prompt_length_signal"`
	MaxPromptBytes              int      `json:"max_prompt_bytes"`
	MaxPromptScanBytes          int      `json:"max_prompt_scan_bytes"`
	EnableContextGuard          bool     `json:"enable_context_guard"`
	MaxContextMessages          int      `json:"max_context_messages"`
	MaxContextScanBytes         int      `json:"max_context_scan_bytes"`
	ExemptVirtualKeyIDs         []string `json:"exempt_virtual_key_ids,omitempty"`
}

type runtimeConfig struct {
	alertThreshold              int
	warmupRequests              int
	window                      time.Duration
	profileTTL                  time.Duration
	maxProfiles                 int
	maxEventsPerProfile         int
	enableCrossKeyCorrelation   bool
	maxDistinctKeysPerIdentity  int
	correlationWindow           time.Duration
	enablePaymentVelocity       bool
	enablePaymentLocation       bool
	paymentWindow               time.Duration
	paymentProfileTTL           time.Duration
	maxPaymentSubjects          int
	maxPaymentEventsPerSubject  int
	maxPaymentsPerWindow        int
	paymentLocationWarmupEvents int
	enableExpiredKeySignal      bool
	enableRevokedKeySignal      bool
	enableExposedKeySignal      bool
	enablePromptOverrideSignals bool
	enablePromptLengthSignal    bool
	maxPromptBytes              int
	maxPromptScanBytes          int
	enableContextGuard          bool
	maxContextMessages          int
	maxContextScanBytes         int
	exemptVirtualKeyIDs         []string
}

// RuntimeConfigStatus exposes reload health without returning config values,
// exemption IDs, or a digest of the config file.
type RuntimeConfigStatus struct {
	Generation      uint64    `json:"generation"`
	LastAppliedAt   time.Time `json:"last_applied_at,omitempty"`
	LastAttemptAt   time.Time `json:"last_attempt_at,omitempty"`
	LastAttemptFail bool      `json:"last_attempt_failed"`
	LastErrorCode   string    `json:"last_error_code,omitempty"`
}

// ConfigReloader validates a complete config snapshot before applying it under
// the engine lock. Invalid updates leave the last-known-good settings in place.
type ConfigReloader struct {
	engine *Engine
	mu     sync.Mutex

	lastDigest      [32]byte
	generation      uint64
	lastAppliedAt   time.Time
	lastAttemptAt   time.Time
	lastAttemptFail bool
	lastErrorCode   string
}

func NewConfigReloader(engine *Engine) (*ConfigReloader, error) {
	if engine == nil {
		return nil, errors.New("fraudguard: config reloader requires an engine")
	}
	return &ConfigReloader{engine: engine}, nil
}

// ReloadJSON atomically applies a valid full snapshot. It returns an error for
// invalid data and does not change the live configuration in that case.
func (r *ConfigReloader) ReloadJSON(data []byte) error {
	if r == nil || r.engine == nil {
		return errors.New("fraudguard: config reloader is not initialized")
	}
	if len(data) == 0 || len(data) > maxRuntimeConfigBytes {
		r.recordFailure("invalid_config_size")
		return fmt.Errorf("fraudguard: runtime config must be between 1 and %d bytes", maxRuntimeConfigBytes)
	}
	settings, err := decodeRuntimeConfig(data)
	if err != nil {
		r.recordFailure("invalid_config")
		return err
	}
	digest := sha256.Sum256(data)
	now := time.Now().UTC()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastAttemptAt = now
	r.lastAttemptFail = false
	r.lastErrorCode = ""
	if r.generation > 0 && digest == r.lastDigest {
		return nil
	}
	r.engine.applyRuntimeConfig(settings)
	r.lastDigest = digest
	r.generation++
	r.lastAppliedAt = now
	return nil
}

// ReloadFile reads a bounded local config file. A failed read or parse preserves
// the last-known-good engine configuration.
func (r *ConfigReloader) ReloadFile(path string) error {
	if strings.TrimSpace(path) == "" {
		r.recordFailure("empty_config_path")
		return errors.New("fraudguard: runtime config path cannot be empty")
	}
	file, err := os.Open(path)
	if err != nil {
		r.recordFailure("config_read_failed")
		return errors.New("fraudguard: unable to read runtime config file")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxRuntimeConfigBytes+1))
	if err != nil {
		r.recordFailure("config_read_failed")
		return errors.New("fraudguard: unable to read runtime config file")
	}
	if len(data) > maxRuntimeConfigBytes {
		r.recordFailure("config_too_large")
		return fmt.Errorf("fraudguard: runtime config exceeds %d bytes", maxRuntimeConfigBytes)
	}
	return r.ReloadJSON(data)
}

// WatchFile applies the current file before starting a bounded polling watcher.
// Callers should cancel ctx and drain the returned error channel during shutdown.
func (r *ConfigReloader) WatchFile(ctx context.Context, path string, interval time.Duration) (<-chan error, error) {
	if interval < 100*time.Millisecond || interval > time.Hour {
		return nil, errors.New("fraudguard: config poll interval must be between 100ms and 1h")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := r.ReloadFile(path); err != nil {
		return nil, err
	}

	errorsOut := make(chan error, 1)
	go func() {
		defer close(errorsOut)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.ReloadFile(path); err != nil {
					select {
					case errorsOut <- err:
					default:
					}
				}
			}
		}
	}()
	return errorsOut, nil
}

func (r *ConfigReloader) Status() RuntimeConfigStatus {
	if r == nil {
		return RuntimeConfigStatus{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return RuntimeConfigStatus{
		Generation:      r.generation,
		LastAppliedAt:   r.lastAppliedAt,
		LastAttemptAt:   r.lastAttemptAt,
		LastAttemptFail: r.lastAttemptFail,
		LastErrorCode:   r.lastErrorCode,
	}
}

func (r *ConfigReloader) recordFailure(code string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.lastAttemptAt = time.Now().UTC()
	r.lastAttemptFail = true
	r.lastErrorCode = code
	r.mu.Unlock()
}

func decodeRuntimeConfig(data []byte) (runtimeConfig, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return runtimeConfig{}, fmt.Errorf("fraudguard: invalid runtime config JSON: %w", err)
	}
	required := []string{
		"version", "alert_threshold", "warmup_requests", "window", "profile_ttl",
		"max_profiles", "max_events_per_profile", "enable_cross_key_correlation",
		"max_distinct_keys_per_identity", "correlation_window", "enable_payment_velocity",
		"enable_payment_location", "payment_window", "payment_profile_ttl",
		"max_payment_subjects", "max_payment_events_per_subject", "max_payments_per_window",
		"payment_location_warmup_events", "enable_expired_key_signal",
		"enable_revoked_key_signal", "enable_exposed_key_signal",
		"enable_prompt_override_signals", "enable_prompt_length_signal",
		"max_prompt_bytes", "max_prompt_scan_bytes",
		"enable_context_guard", "max_context_messages", "max_context_scan_bytes",
	}
	for _, name := range required {
		if _, ok := fields[name]; !ok {
			return runtimeConfig{}, fmt.Errorf("fraudguard: runtime config is missing required field %q", name)
		}
	}

	var document RuntimeConfigDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return runtimeConfig{}, fmt.Errorf("fraudguard: invalid runtime config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return runtimeConfig{}, errors.New("fraudguard: runtime config must contain one JSON object")
	}
	return document.runtimeConfig()
}

func (document RuntimeConfigDocument) runtimeConfig() (runtimeConfig, error) {
	if document.Version != 1 {
		return runtimeConfig{}, errors.New("fraudguard: unsupported runtime config version")
	}
	window, err := parseRuntimeDuration("window", document.Window, time.Second, 24*time.Hour)
	if err != nil {
		return runtimeConfig{}, err
	}
	profileTTL, err := parseRuntimeDuration("profile_ttl", document.ProfileTTL, time.Minute, 30*24*time.Hour)
	if err != nil {
		return runtimeConfig{}, err
	}
	correlationWindow, err := parseRuntimeDuration("correlation_window", document.CorrelationWindow, time.Second, 24*time.Hour)
	if err != nil {
		return runtimeConfig{}, err
	}
	paymentWindow, err := parseRuntimeDuration("payment_window", document.PaymentWindow, time.Second, 24*time.Hour)
	if err != nil {
		return runtimeConfig{}, err
	}
	paymentProfileTTL, err := parseRuntimeDuration("payment_profile_ttl", document.PaymentProfileTTL, time.Minute, 30*24*time.Hour)
	if err != nil {
		return runtimeConfig{}, err
	}

	if document.AlertThreshold < 1 || document.AlertThreshold > 100 {
		return runtimeConfig{}, errors.New("fraudguard: alert_threshold must be between 1 and 100")
	}
	if document.WarmupRequests < 1 || document.WarmupRequests > 10_000 {
		return runtimeConfig{}, errors.New("fraudguard: warmup_requests must be between 1 and 10000")
	}
	if document.MaxProfiles < 1 || document.MaxProfiles > 1_000_000 {
		return runtimeConfig{}, errors.New("fraudguard: max_profiles must be between 1 and 1000000")
	}
	if document.MaxEventsPerProfile < 1 || document.MaxEventsPerProfile > 10_000 {
		return runtimeConfig{}, errors.New("fraudguard: max_events_per_profile must be between 1 and 10000")
	}
	if document.MaxDistinctKeysPerIdentity < 1 || document.MaxDistinctKeysPerIdentity > 100 {
		return runtimeConfig{}, errors.New("fraudguard: max_distinct_keys_per_identity must be between 1 and 100")
	}
	if document.MaxPaymentSubjects < 1 || document.MaxPaymentSubjects > 1_000_000 {
		return runtimeConfig{}, errors.New("fraudguard: max_payment_subjects must be between 1 and 1000000")
	}
	if document.MaxPaymentEventsPerSubject < 1 || document.MaxPaymentEventsPerSubject > 10_000 {
		return runtimeConfig{}, errors.New("fraudguard: max_payment_events_per_subject must be between 1 and 10000")
	}
	if document.MaxPaymentsPerWindow < 1 || document.MaxPaymentsPerWindow > 100_000 {
		return runtimeConfig{}, errors.New("fraudguard: max_payments_per_window must be between 1 and 100000")
	}
	if document.PaymentLocationWarmupEvents < 1 || document.PaymentLocationWarmupEvents > 10_000 {
		return runtimeConfig{}, errors.New("fraudguard: payment_location_warmup_events must be between 1 and 10000")
	}
	if document.MaxPromptBytes < 1 || document.MaxPromptBytes > 4*1024*1024 {
		return runtimeConfig{}, errors.New("fraudguard: max_prompt_bytes must be between 1 and 4194304")
	}
	if document.MaxPromptScanBytes < 1 || document.MaxPromptScanBytes > 1024*1024 {
		return runtimeConfig{}, errors.New("fraudguard: max_prompt_scan_bytes must be between 1 and 1048576")
	}
	if document.MaxContextMessages < 1 || document.MaxContextMessages > maxContextMessages {
		return runtimeConfig{}, errors.New("fraudguard: max_context_messages must be between 1 and 256")
	}
	if document.MaxContextScanBytes < 1 || document.MaxContextScanBytes > maxContextScanBytes {
		return runtimeConfig{}, errors.New("fraudguard: max_context_scan_bytes must be between 1 and 1048576")
	}
	if len(document.ExemptVirtualKeyIDs) > 10_000 {
		return runtimeConfig{}, errors.New("fraudguard: at most 10000 exempt virtual-key IDs may be configured")
	}
	exemptIDs := make([]string, 0, len(document.ExemptVirtualKeyIDs))
	for _, id := range document.ExemptVirtualKeyIDs {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 256 {
			return runtimeConfig{}, errors.New("fraudguard: exempt virtual-key IDs must be 1 to 256 bytes")
		}
		exemptIDs = append(exemptIDs, id)
	}

	return runtimeConfig{
		alertThreshold:              document.AlertThreshold,
		warmupRequests:              document.WarmupRequests,
		window:                      window,
		profileTTL:                  profileTTL,
		maxProfiles:                 document.MaxProfiles,
		maxEventsPerProfile:         document.MaxEventsPerProfile,
		enableCrossKeyCorrelation:   document.EnableCrossKeyCorrelation,
		maxDistinctKeysPerIdentity:  document.MaxDistinctKeysPerIdentity,
		correlationWindow:           correlationWindow,
		enablePaymentVelocity:       document.EnablePaymentVelocity,
		enablePaymentLocation:       document.EnablePaymentLocation,
		paymentWindow:               paymentWindow,
		paymentProfileTTL:           paymentProfileTTL,
		maxPaymentSubjects:          document.MaxPaymentSubjects,
		maxPaymentEventsPerSubject:  document.MaxPaymentEventsPerSubject,
		maxPaymentsPerWindow:        document.MaxPaymentsPerWindow,
		paymentLocationWarmupEvents: document.PaymentLocationWarmupEvents,
		enableExpiredKeySignal:      document.EnableExpiredKeySignal,
		enableRevokedKeySignal:      document.EnableRevokedKeySignal,
		enableExposedKeySignal:      document.EnableExposedKeySignal,
		enablePromptOverrideSignals: document.EnablePromptOverrideSignals,
		enablePromptLengthSignal:    document.EnablePromptLengthSignal,
		maxPromptBytes:              document.MaxPromptBytes,
		maxPromptScanBytes:          document.MaxPromptScanBytes,
		enableContextGuard:          document.EnableContextGuard,
		maxContextMessages:          document.MaxContextMessages,
		maxContextScanBytes:         document.MaxContextScanBytes,
		exemptVirtualKeyIDs:         exemptIDs,
	}, nil
}

func parseRuntimeDuration(name, value string, minimum, maximum time.Duration) (time.Duration, error) {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || duration < minimum || duration > maximum {
		return 0, fmt.Errorf("fraudguard: %s must be between %s and %s", name, minimum, maximum)
	}
	return duration, nil
}
