// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	defaultBudgetEntries = 10_000
	maxBudgetEntries     = 100_000
)

type BudgetMonitorConfig struct {
	MaxEntries int
	Retention  time.Duration
}

// BudgetEvent must come from trusted host-side usage and pricing data. Cost is
// expressed as integer micro-USD; zero means cost is not available.
type BudgetEvent struct {
	TenantID            string
	Period              string
	Tokens              int64
	EstimatedCostMicros int64
	MaxTokens           int64
	MaxCostMicros       int64
	WarningPercent      int
}

// BudgetAssessment is advisory only. It contains no authorization or block
// decision and must not be used to interrupt a model request.
type BudgetAssessment struct {
	Period              string `json:"period"`
	TokensUsed          int64  `json:"tokens_used"`
	EstimatedCostMicros int64  `json:"estimated_cost_micros"`
	MaxTokens           int64  `json:"max_tokens,omitempty"`
	MaxCostMicros       int64  `json:"max_cost_micros,omitempty"`
	TokenWarning        bool   `json:"token_warning"`
	CostWarning         bool   `json:"cost_warning"`
	TokenLimitExceeded  bool   `json:"token_limit_exceeded"`
	CostLimitExceeded   bool   `json:"cost_limit_exceeded"`
	Exceeded            bool   `json:"exceeded"`
	Outcome             string `json:"outcome"`
}

type BudgetMetrics struct {
	TrackedPeriods int    `json:"tracked_periods"`
	Observations   uint64 `json:"observations"`
	Warnings       uint64 `json:"warnings"`
	Exceeded       uint64 `json:"exceeded"`
}

type budgetUsage struct {
	tokens     int64
	costMicros int64
	lastSeen   time.Time
}

// BudgetMonitor stores bounded per-period aggregates keyed by an ephemeral
// HMAC. Restarting the process resets it; hosts needing durable budgets should
// use their trusted billing ledger as the authority.
type BudgetMonitor struct {
	mu        sync.Mutex
	key       [32]byte
	max       int
	retention time.Duration
	usage     map[string]budgetUsage
	metrics   BudgetMetrics
}

func NewBudgetMonitor(config BudgetMonitorConfig) (*BudgetMonitor, error) {
	if config.MaxEntries <= 0 {
		config.MaxEntries = defaultBudgetEntries
	}
	if config.MaxEntries > maxBudgetEntries {
		return nil, errors.New("fraudguard: budget monitor cannot track more than 100000 tenant-periods")
	}
	if config.Retention <= 0 {
		config.Retention = 400 * 24 * time.Hour
	}
	if config.Retention > 400*24*time.Hour {
		return nil, errors.New("fraudguard: budget monitor retention cannot exceed 400 days")
	}
	monitor := &BudgetMonitor{
		max:       config.MaxEntries,
		retention: config.Retention,
		usage:     make(map[string]budgetUsage),
	}
	if _, err := rand.Read(monitor.key[:]); err != nil {
		return nil, errors.New("fraudguard: unable to initialize private budget hashing")
	}
	return monitor, nil
}

func (m *BudgetMonitor) Observe(event BudgetEvent) (BudgetAssessment, error) {
	if m == nil {
		return BudgetAssessment{}, errors.New("fraudguard: budget monitor is not initialized")
	}
	if strings.TrimSpace(event.TenantID) == "" {
		return BudgetAssessment{}, ErrInvalidTenantID
	}
	if !validUsageMonth(event.Period) {
		return BudgetAssessment{}, errors.New("fraudguard: budget period must use YYYY-MM format")
	}
	if event.Tokens < 0 || event.EstimatedCostMicros < 0 || event.MaxTokens < 0 || event.MaxCostMicros < 0 {
		return BudgetAssessment{}, errors.New("fraudguard: budget counters and limits cannot be negative")
	}
	if event.WarningPercent < 0 || event.WarningPercent > 100 {
		return BudgetAssessment{}, errors.New("fraudguard: budget warning percentage must be between 0 and 100")
	}
	warningPercent := event.WarningPercent
	if warningPercent == 0 {
		warningPercent = 80
	}

	now := time.Now()
	key := m.entryKey(event.TenantID, event.Period)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(now)
	entry, found := m.usage[key]
	if !found {
		if len(m.usage) >= m.max {
			m.evictOldestLocked()
		}
		entry = budgetUsage{}
	}
	entry.tokens = saturatingAddNonnegative(entry.tokens, event.Tokens)
	entry.costMicros = saturatingAddNonnegative(entry.costMicros, event.EstimatedCostMicros)
	entry.lastSeen = now
	m.usage[key] = entry

	assessment := BudgetAssessment{
		Period:              event.Period,
		TokensUsed:          entry.tokens,
		EstimatedCostMicros: entry.costMicros,
		MaxTokens:           event.MaxTokens,
		MaxCostMicros:       event.MaxCostMicros,
		Outcome:             "within_budget",
	}
	if event.MaxTokens > 0 {
		assessment.TokenWarning = reachedPercent(entry.tokens, event.MaxTokens, warningPercent)
		assessment.TokenLimitExceeded = entry.tokens > event.MaxTokens
	}
	if event.MaxCostMicros > 0 {
		assessment.CostWarning = reachedPercent(entry.costMicros, event.MaxCostMicros, warningPercent)
		assessment.CostLimitExceeded = entry.costMicros > event.MaxCostMicros
	}
	assessment.Exceeded = assessment.TokenLimitExceeded || assessment.CostLimitExceeded
	switch {
	case assessment.Exceeded:
		assessment.Outcome = "budget_exceeded_observe_only"
		m.metrics.Exceeded++
	case assessment.TokenWarning || assessment.CostWarning:
		assessment.Outcome = "budget_warning_observe_only"
		m.metrics.Warnings++
	}
	m.metrics.Observations++
	m.metrics.TrackedPeriods = len(m.usage)
	return assessment, nil
}

func (m *BudgetMonitor) Metrics() BudgetMetrics {
	if m == nil {
		return BudgetMetrics{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics.TrackedPeriods = len(m.usage)
	return m.metrics
}

func (m *BudgetMonitor) Prune(now time.Time) int {
	if m == nil {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := m.pruneLocked(now)
	m.metrics.TrackedPeriods = len(m.usage)
	return removed
}

func (m *BudgetMonitor) pruneLocked(now time.Time) int {
	removed := 0
	for key, entry := range m.usage {
		if now.Sub(entry.lastSeen) >= m.retention {
			delete(m.usage, key)
			removed++
		}
	}
	return removed
}

func (m *BudgetMonitor) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	for key, entry := range m.usage {
		if oldestKey == "" || entry.lastSeen.Before(oldest) {
			oldestKey = key
			oldest = entry.lastSeen
		}
	}
	if oldestKey != "" {
		delete(m.usage, oldestKey)
	}
}

func (m *BudgetMonitor) entryKey(tenantID, period string) string {
	mac := hmac.New(sha256.New, m.key[:])
	_, _ = mac.Write([]byte("fraudguard:budget:v1:" + strings.TrimSpace(tenantID) + ":" + period))
	return hex.EncodeToString(mac.Sum(nil))
}

func (p *Plugin) SetBudgetMonitor(monitor *BudgetMonitor) error {
	if p == nil {
		return errors.New("fraudguard: plugin is not initialized")
	}
	p.budgetMu.Lock()
	defer p.budgetMu.Unlock()
	if p.budget != nil {
		return errors.New("fraudguard: budget monitor is already configured")
	}
	if monitor == nil {
		return errors.New("fraudguard: budget monitor is required")
	}
	p.budget = monitor
	return nil
}

func (p *Plugin) ObserveBudget(event BudgetEvent) (BudgetAssessment, error) {
	if p == nil {
		return BudgetAssessment{}, errors.New("fraudguard: plugin is not initialized")
	}
	p.budgetMu.RLock()
	monitor := p.budget
	p.budgetMu.RUnlock()
	if monitor == nil {
		return BudgetAssessment{}, errors.New("fraudguard: budget monitor is not configured")
	}
	return monitor.Observe(event)
}

func (p *Plugin) BudgetMetrics() BudgetMetrics {
	if p == nil {
		return BudgetMetrics{}
	}
	p.budgetMu.RLock()
	monitor := p.budget
	p.budgetMu.RUnlock()
	return monitor.Metrics()
}
