// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0
//
// Package fraudguard provides an observe-only behavioral anomaly detector for Bifrost.
//
// The detector keeps bounded, in-memory behavioral summaries per resolved virtual key.
// It never retains prompts, payloads, raw virtual-key IDs, or IP addresses.
package fraudguard

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// Config controls the detector. Zero values are replaced with conservative defaults.
type Config struct {
	// WarmupRequests is the minimum number of prior observations before scoring starts.
	WarmupRequests int
	// Window bounds the rolling behavioral history used for scoring.
	Window time.Duration
	// ProfileTTL removes a profile after it has been idle for this long.
	ProfileTTL time.Duration
	// MaxProfiles and MaxEventsPerProfile bound memory use.
	MaxProfiles         int
	MaxEventsPerProfile int
	// EnableCrossKeyCorrelation opts in to grouping keys by a trusted cohort ID.
	// Disabled by default; no cohort state is created unless explicitly enabled.
	EnableCrossKeyCorrelation bool
	// MaxDistinctKeysPerIdentity triggers key_rotation when exceeded (default 3, max 100).
	MaxDistinctKeysPerIdentity int
	// CorrelationWindow bounds the distinct-key set (defaults to Window).
	CorrelationWindow time.Duration
	// Payment signals are independently opt-in and use only trusted events supplied
	// by the host integration. No payment data is inferred from request headers.
	EnablePaymentVelocity       bool
	EnablePaymentLocation       bool
	PaymentWindow               time.Duration
	PaymentProfileTTL           time.Duration
	MaxPaymentSubjects          int
	MaxPaymentEventsPerSubject  int
	MaxPaymentsPerWindow        int
	PaymentLocationWarmupEvents int
	// Credential-state signals are independently opt-in. The host must supply
	// authoritative results from its auth or credential-management layer.
	EnableExpiredKeySignal bool
	EnableRevokedKeySignal bool
	EnableExposedKeySignal bool
	// Prompt checks inspect user text transiently and default off. Only boolean
	// indicators and the byte count are passed to Start; raw text is never stored.
	EnablePromptOverrideSignals bool
	EnablePromptLengthSignal    bool
	MaxPromptBytes              int
	MaxPromptScanBytes          int
	// Custom prompt rules are trusted, operator-supplied RE2 expressions.
	// Matches add an advisory signal only and default off.
	EnableCustomPromptRules bool
	CustomPromptRules       *CustomPromptRules
	// EnableContextGuard opts in to bounded, transient inspection of user turns.
	// The guard returns only fixed signal booleans and never stores message text.
	EnableContextGuard  bool
	MaxContextMessages  int
	MaxContextScanBytes int
	// ThreatIntelFeed is a trusted, in-memory indicator set. Matches only add
	// an advisory signal and never block a request.
	ThreatIntelFeed *ThreatIntelFeed
	// AlertThreshold is inclusive; findings below it remain observable but are not alerts.
	AlertThreshold int
	// AI review is opt-in. The host supplies an adapter backed by a separately
	// configured Bifrost model client; the engine only sends sanitized findings.
	EnableAIAnalysis    bool
	AIAnalyzer          AIAnalyzer
	OnAIReview          func(AIReviewEvent)
	AIAnalysisMinScore  int
	AIAnalysisTimeout   time.Duration
	AIAnalysisWorkers   int
	AIAnalysisQueueSize int
	// ExemptVirtualKeyIDs contains resolved internal IDs, never credential values.
	// IDs are immediately HMACed and are not stored in clear text.
	ExemptVirtualKeyIDs []string
	// OnFinding receives structured, payload-free findings through a bounded
	// asynchronous queue. Queue overflow drops the callback event and increments a metric.
	OnFinding func(Finding)
	// FindingCallbackQueueSize bounds pending audit callbacks (default 256, maximum 4096).
	FindingCallbackQueueSize int
}

// Signal is a deterministic, explainable component of a risk score.
// It intentionally has no model, provider, key, prompt, or request identifiers.
type Signal struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
	Weight int    `json:"weight"`
}

// Finding reports a score and the named behavioral signals behind it.
type Finding struct {
	Timestamp time.Time `json:"timestamp"`
	Score     int       `json:"score"`
	Threshold int       `json:"threshold"`
	Alert     bool      `json:"alert"`
	Warmup    bool      `json:"warmup"`
	Signals   []Signal  `json:"signals"`
}

// Request is the only data needed to evaluate a request start. VirtualKeyID must
// be Bifrost's resolved internal virtual-key ID, not a raw bearer credential.
type Request struct {
	VirtualKeyID string
	// CohortID is an optional trusted, stable identity for privacy-aware key correlation.
	// It is immediately HMACed and never retained in clear text.
	CohortID  string
	Provider  string
	Model     string
	Timestamp time.Time
	// Prompt indicators are transient hook results. The engine never stores them
	// in profile history; callers must not put raw prompt text here.
	PromptOverrideDetected      bool
	CustomPromptRuleMatched     bool
	PromptBytes                 int64
	ContextProgressiveJailbreak bool
	ContextDataExfiltration     bool
	ContextSchemaManipulation   bool
	// AlertThresholdOverride is supplied only by a trusted tenant-plan resolver.
	// It changes alert classification, never the score or request handling.
	AlertThresholdOverride *int
	// AIReviewOverride can disable optional advisory review for a plan. It can
	// never enable AI review when the engine's operator configuration disables it.
	AIReviewOverride *bool
	// CredentialStatus must come from a trusted authentication or credential source.
	CredentialStatus CredentialStatus
}

// CredentialStatus contains explicit results from a trusted key-management source.
// FraudGuard does not inspect credentials or determine these states itself.
type CredentialStatus struct {
	Expired bool
	Revoked bool
	Exposed bool
}

// PaymentEvent is a minimal, trusted payment signal. Send one event per transaction
// attempt. SubjectID should be a stable internal or pseudonymous account identifier.
// CountryCode must be a coarse country code from a trusted payment source, never a
// value inferred from caller headers. Raw identifiers and country codes are not
// retained by the engine.
type PaymentEvent struct {
	SubjectID                    string
	CountryCode                  string
	Timestamp                    time.Time
	TenantID                     string `json:"-"`
	MaxPaymentsPerWindowOverride int
	AlertThresholdOverride       *int
	AIReviewOverride             *bool
}

// Completion contains request outcome metadata. It must not contain response bodies
// or error text; only token totals and whether the call failed are used.
type Completion struct {
	InputTokens  int64
	OutputTokens int64
	Failed       bool
	Timestamp    time.Time
}

// Handle correlates a request's post-hook with its pre-hook. Its fields are private
// and contain only a process-keyed HMAC profile digest and a sequence number.
type Handle struct {
	profileID       string
	sequence        uint64
	threshold       int
	aiReviewAllowed bool
}

// Assessment is the current score and its explanation.
type Assessment struct {
	Finding Finding
}

// Metrics is a privacy-preserving aggregate snapshot.
type Metrics struct {
	Profiles                  int    `json:"profiles"`
	Cohorts                   int    `json:"cohorts"`
	PaymentSubjects           int    `json:"payment_subjects"`
	Requests                  uint64 `json:"requests"`
	PaymentEvents             uint64 `json:"payment_events"`
	Findings                  uint64 `json:"findings"`
	Alerts                    uint64 `json:"alerts"`
	ExemptRequests            uint64 `json:"exempt_requests"`
	AIReviewsQueued           uint64 `json:"ai_reviews_queued"`
	AIReviewsDropped          uint64 `json:"ai_reviews_dropped"`
	AIReviewsCompleted        uint64 `json:"ai_reviews_completed"`
	AIReviewsFailed           uint64 `json:"ai_reviews_failed"`
	FindingCallbacksQueued    uint64 `json:"finding_callbacks_queued"`
	FindingCallbacksDropped   uint64 `json:"finding_callbacks_dropped"`
	FindingCallbacksCompleted uint64 `json:"finding_callbacks_completed"`
	FindingCallbacksFailed    uint64 `json:"finding_callbacks_failed"`
}

type event struct {
	sequence uint64
	at       time.Time
	provider string
	model    string
	input    int64
	output   int64
	complete bool
	failed   bool
}

type profile struct {
	events   []event
	lastSeen time.Time
}

type cohortProfile struct {
	keys     map[string]time.Time
	lastSeen time.Time
	alerted  bool
}

type paymentObservation struct {
	at            time.Time
	countryDigest string
}

type paymentProfile struct {
	events          []paymentObservation
	lastSeen        time.Time
	velocityAlerted bool
}

// Engine owns the thread-safe profile store and deterministic scoring logic.
type Engine struct {
	mu            sync.Mutex
	cfg           Config
	hashKey       [32]byte
	exemptions    map[string]struct{}
	profiles      map[string]*profile
	cohorts       map[string]*cohortProfile
	payments      map[string]*paymentProfile
	nextID        uint64
	metrics       Metrics
	aiQueue       chan aiAnalysisJob
	aiMu          sync.Mutex
	aiClosed      bool
	aiWG          sync.WaitGroup
	findingQueue  chan Finding
	findingMu     sync.Mutex
	findingClosed bool
	findingWG     sync.WaitGroup
}

// NewEngine creates an in-memory detector. A random process-local HMAC key ensures
// profile map keys cannot be reversed or correlated across restarts.
func NewEngine(config Config) (*Engine, error) {
	if config.EnableCustomPromptRules && config.CustomPromptRules == nil {
		return nil, errors.New("fraudguard: custom prompt rules enabled without a rule set")
	}
	config = withDefaults(config)
	if err := ValidateStartupSecurity(config); err != nil {
		return nil, err
	}
	if config.EnableAIAnalysis && config.AIAnalyzer == nil {
		return nil, errors.New("fraudguard: AI analysis enabled without a host model adapter")
	}
	if config.EnableAIAnalysis && config.OnAIReview == nil {
		return nil, errors.New("fraudguard: AI analysis enabled without an AI review callback")
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, errors.New("fraudguard: unable to initialize private profile hashing")
	}
	engine := &Engine{
		cfg:        config,
		hashKey:    secret,
		exemptions: make(map[string]struct{}, len(config.ExemptVirtualKeyIDs)),
		profiles:   make(map[string]*profile),
		cohorts:    make(map[string]*cohortProfile),
		payments:   make(map[string]*paymentProfile),
	}
	for _, id := range config.ExemptVirtualKeyIDs {
		if strings.TrimSpace(id) != "" {
			engine.exemptions[engine.profileDigest(id)] = struct{}{}
		}
	}
	// Keep only digests; the caller-provided identifiers must not remain in config.
	engine.cfg.ExemptVirtualKeyIDs = nil
	if config.OnFinding != nil {
		engine.findingQueue = make(chan Finding, config.FindingCallbackQueueSize)
		engine.findingWG.Add(1)
		go engine.runFindingWorker()
	}
	if config.EnableAIAnalysis {
		engine.aiQueue = make(chan aiAnalysisJob, config.AIAnalysisQueueSize)
		for i := 0; i < config.AIAnalysisWorkers; i++ {
			engine.aiWG.Add(1)
			go engine.runAIWorker()
		}
	}
	return engine, nil
}

func withDefaults(config Config) Config {
	if config.WarmupRequests <= 0 {
		config.WarmupRequests = 5
	}
	if config.Window <= 0 {
		config.Window = 15 * time.Minute
	}
	if config.ProfileTTL <= 0 {
		config.ProfileTTL = 2 * time.Hour
	}
	if config.MaxProfiles <= 0 {
		config.MaxProfiles = 10_000
	}
	if config.MaxEventsPerProfile <= 0 {
		config.MaxEventsPerProfile = 256
	}
	if config.MaxDistinctKeysPerIdentity <= 0 {
		config.MaxDistinctKeysPerIdentity = 3
	}
	if config.MaxDistinctKeysPerIdentity > 100 {
		config.MaxDistinctKeysPerIdentity = 100
	}
	if config.CorrelationWindow <= 0 {
		config.CorrelationWindow = config.Window
	}
	if config.PaymentWindow <= 0 {
		config.PaymentWindow = config.Window
	}
	if config.PaymentProfileTTL <= 0 {
		config.PaymentProfileTTL = config.ProfileTTL
	}
	if config.MaxPaymentSubjects <= 0 {
		config.MaxPaymentSubjects = config.MaxProfiles
	}
	if config.MaxPaymentEventsPerSubject <= 0 {
		config.MaxPaymentEventsPerSubject = config.MaxEventsPerProfile
	}
	if config.MaxPaymentEventsPerSubject > 10_000 {
		config.MaxPaymentEventsPerSubject = 10_000
	}
	if config.MaxPaymentsPerWindow <= 0 {
		config.MaxPaymentsPerWindow = 10
	}
	if config.PaymentLocationWarmupEvents <= 0 {
		config.PaymentLocationWarmupEvents = 3
	}
	if config.AlertThreshold <= 0 {
		config.AlertThreshold = 60
	}
	if config.AlertThreshold > 100 {
		config.AlertThreshold = 100
	}
	if config.MaxPromptBytes <= 0 {
		config.MaxPromptBytes = 32_768
	}
	if config.MaxPromptBytes > 4*1024*1024 {
		config.MaxPromptBytes = 4 * 1024 * 1024
	}
	if config.MaxPromptScanBytes <= 0 {
		config.MaxPromptScanBytes = 64 * 1024
	}
	if config.MaxPromptScanBytes > 1024*1024 {
		config.MaxPromptScanBytes = 1024 * 1024
	}
	if config.MaxContextMessages <= 0 {
		config.MaxContextMessages = defaultContextMessages
	}
	if config.MaxContextMessages > maxContextMessages {
		config.MaxContextMessages = maxContextMessages
	}
	if config.MaxContextScanBytes <= 0 {
		config.MaxContextScanBytes = defaultContextScanBytes
	}
	if config.MaxContextScanBytes > maxContextScanBytes {
		config.MaxContextScanBytes = maxContextScanBytes
	}
	if config.FindingCallbackQueueSize <= 0 {
		config.FindingCallbackQueueSize = 256
	}
	if config.FindingCallbackQueueSize > 4096 {
		config.FindingCallbackQueueSize = 4096
	}
	if config.AIAnalysisMinScore <= 0 {
		config.AIAnalysisMinScore = config.AlertThreshold
	}
	if config.AIAnalysisMinScore > 100 {
		config.AIAnalysisMinScore = 100
	}
	if config.AIAnalysisTimeout <= 0 {
		config.AIAnalysisTimeout = 5 * time.Second
	}
	if config.AIAnalysisWorkers <= 0 {
		config.AIAnalysisWorkers = 1
	}
	if config.AIAnalysisWorkers > 8 {
		config.AIAnalysisWorkers = 8
	}
	if config.AIAnalysisQueueSize <= 0 {
		config.AIAnalysisQueueSize = 64
	}
	if config.AIAnalysisQueueSize > 4096 {
		config.AIAnalysisQueueSize = 4096
	}
	return config
}

// Start records a request start and assesses provider/model novelty and unusually
// fast request timing. The returned handle must be passed to Complete when available.
func (e *Engine) Start(request Request) (Handle, Assessment) {
	now := request.Timestamp
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	if strings.TrimSpace(request.VirtualKeyID) == "" {
		return Handle{}, Assessment{Finding: e.emptyFinding(now, false)}
	}
	id := e.profileDigest(request.VirtualKeyID)
	provider := safeLabel(request.Provider)
	model := safeLabel(request.Model)

	e.mu.Lock()
	threshold := e.cfg.AlertThreshold
	if request.AlertThresholdOverride != nil && *request.AlertThresholdOverride >= 0 && *request.AlertThresholdOverride <= 100 {
		threshold = *request.AlertThresholdOverride
	}
	aiReviewAllowed := e.cfg.EnableAIAnalysis
	if request.AIReviewOverride != nil {
		aiReviewAllowed = aiReviewAllowed && *request.AIReviewOverride
	}
	if _, exempt := e.exemptions[id]; exempt {
		e.metrics.ExemptRequests++
		e.mu.Unlock()
		return Handle{}, Assessment{Finding: e.emptyFinding(now, false)}
	}
	var signals []Signal
	if e.cfg.ThreatIntelFeed != nil && e.cfg.ThreatIntelFeed.MatchInternalKeyID(request.VirtualKeyID, now) {
		signals = append(signals, Signal{
			Name:   "trusted_threat_indicator",
			Reason: "trusted local threat feed matched this resolved internal virtual key",
			Weight: 60,
		})
	}
	if e.cfg.EnableExpiredKeySignal && request.CredentialStatus.Expired {
		signals = append(signals, Signal{
			Name:   "key_expired",
			Reason: "trusted credential status marks this virtual key as expired",
			Weight: 70,
		})
	}
	if e.cfg.EnableRevokedKeySignal && request.CredentialStatus.Revoked {
		signals = append(signals, Signal{
			Name:   "key_revoked",
			Reason: "trusted credential status marks this virtual key as revoked",
			Weight: 70,
		})
	}
	if e.cfg.EnableExposedKeySignal && request.CredentialStatus.Exposed {
		signals = append(signals, Signal{
			Name:   "key_exposed",
			Reason: "trusted credential status marks this virtual key as exposed",
			Weight: 70,
		})
	}
	if e.cfg.EnablePromptOverrideSignals && request.PromptOverrideDetected {
		signals = append(signals, Signal{
			Name:   "prompt_override_phrase",
			Reason: "user text matched a local override or jailbreak phrase heuristic",
			Weight: 40,
		})
	}
	if e.cfg.EnableCustomPromptRules && request.CustomPromptRuleMatched {
		signals = append(signals, Signal{
			Name:   "custom_prompt_pattern",
			Reason: "user text matched an operator-configured advisory pattern",
			Weight: 35,
		})
	}
	if e.cfg.EnablePromptLengthSignal && request.PromptBytes > int64(e.cfg.MaxPromptBytes) {
		signals = append(signals, Signal{
			Name:   "prompt_size_spike",
			Reason: "user text exceeded the configured prompt byte threshold",
			Weight: 25,
		})
	}
	if e.cfg.EnableContextGuard && request.ContextProgressiveJailbreak {
		signals = append(signals, Signal{
			Name:   "context_progressive_jailbreak",
			Reason: "bounded user-turn history matched a staged instruction-override heuristic",
			Weight: 35,
		})
	}
	if e.cfg.EnableContextGuard && request.ContextDataExfiltration {
		signals = append(signals, Signal{
			Name:   "context_data_exfiltration",
			Reason: "bounded user text matched a sensitive-information disclosure heuristic",
			Weight: 45,
		})
	}
	if e.cfg.EnableContextGuard && request.ContextSchemaManipulation {
		signals = append(signals, Signal{
			Name:   "context_schema_manipulation",
			Reason: "bounded user text paired schema markers with an override directive",
			Weight: 35,
		})
	}
	if e.cfg.EnableCrossKeyCorrelation && request.CohortID != "" {
		cohortID := e.cohortDigest(request.CohortID)
		if e.trackCohortKeyLocked(cohortID, id, now) {
			signals = append(signals, Signal{
				Name:   "key_rotation",
				Reason: "this authenticated cohort used more distinct virtual keys than configured",
				Weight: 30,
			})
		}
	}
	p := e.getOrCreateProfileLocked(id, now)
	e.pruneEventsLocked(p, now)
	warmup := len(p.events) < e.cfg.WarmupRequests
	if signals == nil {
		signals = make([]Signal, 0, 3)
	}
	if !warmup {
		if !hasLabel(p.events, provider, true) && provider != "" {
			signals = append(signals, Signal{
				Name:   "new_provider",
				Reason: "provider is outside this key's observed provider mix",
				Weight: 25,
			})
		}
		if !hasLabel(p.events, model, false) && model != "" {
			signals = append(signals, Signal{
				Name:   "new_model",
				Reason: "model is outside this key's observed model mix",
				Weight: 25,
			})
		}
		if isTimingAnomaly(p.events, now, e.cfg.Window) {
			signals = append(signals, Signal{
				Name:   "rapid_sequence",
				Reason: "request arrived much sooner than this key's typical interval",
				Weight: 30,
			})
		}
	}

	e.nextID++
	sequence := e.nextID
	newEvent := event{
		sequence: sequence,
		at:       now,
		provider: provider,
		model:    model,
	}
	if len(p.events) > e.cfg.MaxEventsPerProfile {
		p.events = p.events[len(p.events)-e.cfg.MaxEventsPerProfile:]
	}
	if len(p.events) < e.cfg.MaxEventsPerProfile {
		p.events = append(p.events, newEvent)
	} else {
		copy(p.events, p.events[1:])
		p.events[len(p.events)-1] = newEvent
	}
	p.lastSeen = now
	e.metrics.Requests++
	finding := e.makeFindingAtThresholdLocked(now, signals, warmup, threshold)
	sink := e.cfg.OnFinding
	e.mu.Unlock()
	e.emit(sink, finding, aiReviewAllowed)

	return Handle{profileID: id, sequence: sequence, threshold: threshold, aiReviewAllowed: aiReviewAllowed}, Assessment{Finding: finding}
}

// Complete records response usage and failure outcome, then scores token-volume
// spikes and repeated failures. Error messages and response content are never read.
func (e *Engine) Complete(handle Handle, completion Completion) Assessment {
	now := completion.Timestamp
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	if handle.profileID == "" || handle.sequence == 0 {
		return Assessment{Finding: e.emptyFinding(now, false)}
	}

	input := nonnegative(completion.InputTokens)
	output := nonnegative(completion.OutputTokens)
	e.mu.Lock()
	p := e.profiles[handle.profileID]
	if p == nil {
		e.mu.Unlock()
		return Assessment{Finding: e.emptyFinding(now, false)}
	}
	e.pruneEventsLocked(p, now)
	idx := eventIndex(p.events, handle.sequence)
	if idx < 0 {
		e.mu.Unlock()
		return Assessment{Finding: e.emptyFinding(now, false)}
	}

	current := p.events[idx]
	warmup := completedCount(p.events, handle.sequence) < e.cfg.WarmupRequests
	signals := make([]Signal, 0, 3)
	if !warmup {
		priorInput := tokenValues(p.events, handle.sequence, true)
		priorOutput := tokenValues(p.events, handle.sequence, false)
		if isTokenSpike(priorInput, input, 512) {
			signals = append(signals, Signal{
				Name:   "input_volume_spike",
				Reason: "input token volume is substantially above this key's completed-request baseline",
				Weight: 30,
			})
		}
		if isTokenSpike(priorOutput, output, 256) {
			signals = append(signals, Signal{
				Name:   "output_volume_spike",
				Reason: "output token volume is substantially above this key's completed-request baseline",
				Weight: 25,
			})
		}
		if isRepeatedFailure(p.events, handle.sequence, completion.Failed, now, e.cfg.Window) {
			signals = append(signals, Signal{
				Name:   "repeated_errors",
				Reason: "recent failures dominate this key's rolling request outcomes",
				Weight: 35,
			})
		}
	}

	current.input = input
	current.output = output
	current.complete = true
	current.failed = completion.Failed
	p.events[idx] = current
	p.lastSeen = now
	threshold := handle.threshold
	if threshold < 0 || threshold > 100 {
		threshold = e.cfg.AlertThreshold
	}
	finding := e.makeFindingAtThresholdLocked(now, signals, warmup, threshold)
	sink := e.cfg.OnFinding
	e.mu.Unlock()
	e.emit(sink, finding, handle.aiReviewAllowed)
	return Assessment{Finding: finding}
}

// RecordPayment evaluates one trusted payment event. Call it once per transaction
// attempt; it stores only a keyed digest of the subject and coarse country.
// It has no blocking or authorization behavior.
func (e *Engine) RecordPayment(payment PaymentEvent) Assessment {
	now := payment.Timestamp
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	subjectID := strings.TrimSpace(payment.SubjectID)
	if subjectID == "" {
		return Assessment{Finding: e.emptyFinding(now, false)}
	}

	subjectDigest := e.digest("payment-subject", subjectID)
	e.mu.Lock()
	if !e.cfg.EnablePaymentVelocity && !e.cfg.EnablePaymentLocation {
		e.mu.Unlock()
		return Assessment{Finding: e.emptyFinding(now, false)}
	}
	countryDigest := ""
	if e.cfg.EnablePaymentLocation {
		if country := normalizeCountryCode(payment.CountryCode); country != "" {
			countryDigest = e.digest("payment-country", country)
		}
	}
	maxPayments := e.cfg.MaxPaymentsPerWindow
	if payment.MaxPaymentsPerWindowOverride > 0 {
		maxPayments = min(payment.MaxPaymentsPerWindowOverride, e.cfg.MaxPaymentEventsPerSubject)
	}
	threshold := e.cfg.AlertThreshold
	if payment.AlertThresholdOverride != nil && *payment.AlertThresholdOverride >= 0 && *payment.AlertThresholdOverride <= 100 {
		threshold = *payment.AlertThresholdOverride
	}
	aiReviewAllowed := e.cfg.EnableAIAnalysis
	if payment.AIReviewOverride != nil {
		aiReviewAllowed = aiReviewAllowed && *payment.AIReviewOverride
	}
	p := e.getOrCreatePaymentProfileLocked(subjectDigest, now)
	e.prunePaymentEventsLocked(p, now)
	signals := make([]Signal, 0, 2)
	if e.cfg.EnablePaymentVelocity {
		if len(p.events) <= maxPayments {
			p.velocityAlerted = false
		}
		if len(p.events)+1 > maxPayments && !p.velocityAlerted {
			signals = append(signals, Signal{
				Name:   "payment_velocity",
				Reason: "trusted payment-event count exceeded the configured rolling-window limit",
				Weight: 60,
			})
			p.velocityAlerted = true
		}
	}
	if e.cfg.EnablePaymentLocation && countryDigest != "" &&
		countPaymentLocations(p.events) >= e.cfg.PaymentLocationWarmupEvents &&
		!hasPaymentLocation(p.events, countryDigest) {
		signals = append(signals, Signal{
			Name:   "payment_new_location",
			Reason: "trusted coarse payment country is outside this subject's recent locations",
			Weight: 35,
		})
	}
	newObservation := paymentObservation{at: now, countryDigest: countryDigest}
	if len(p.events) > e.cfg.MaxPaymentEventsPerSubject {
		p.events = p.events[len(p.events)-e.cfg.MaxPaymentEventsPerSubject:]
	}
	if len(p.events) < e.cfg.MaxPaymentEventsPerSubject {
		p.events = append(p.events, newObservation)
	} else {
		copy(p.events, p.events[1:])
		p.events[len(p.events)-1] = newObservation
	}
	p.lastSeen = now
	e.metrics.PaymentEvents++
	finding := e.makeFindingAtThresholdLocked(now, signals, false, threshold)
	sink := e.cfg.OnFinding
	e.mu.Unlock()
	e.emit(sink, finding, aiReviewAllowed)
	return Assessment{Finding: finding}
}

// Prune expires idle profiles and correlation cohorts. It returns the number of
// stored records removed (request profiles, cohorts, plus payment profiles).
func (e *Engine) Prune(now time.Time) int {
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pruneProfilesLocked(now) + e.pruneCohortsLocked(now) + e.prunePaymentProfilesLocked(now)
}

// Metrics returns aggregate counters without exposing individual profile identifiers.
func (e *Engine) Metrics() Metrics {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.metrics
	out.Profiles = len(e.profiles)
	out.Cohorts = len(e.cohorts)
	out.PaymentSubjects = len(e.payments)
	return out
}

func (e *Engine) configSnapshot() Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	config := e.cfg
	config.ExemptVirtualKeyIDs = nil
	return config
}

func (e *Engine) applyRuntimeConfig(settings runtimeConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.cfg.AlertThreshold = settings.alertThreshold
	e.cfg.WarmupRequests = settings.warmupRequests
	e.cfg.Window = settings.window
	e.cfg.ProfileTTL = settings.profileTTL
	e.cfg.MaxProfiles = settings.maxProfiles
	e.cfg.MaxEventsPerProfile = settings.maxEventsPerProfile
	e.cfg.EnableCrossKeyCorrelation = settings.enableCrossKeyCorrelation
	e.cfg.MaxDistinctKeysPerIdentity = settings.maxDistinctKeysPerIdentity
	e.cfg.CorrelationWindow = settings.correlationWindow
	e.cfg.EnablePaymentVelocity = settings.enablePaymentVelocity
	e.cfg.EnablePaymentLocation = settings.enablePaymentLocation
	e.cfg.PaymentWindow = settings.paymentWindow
	e.cfg.PaymentProfileTTL = settings.paymentProfileTTL
	e.cfg.MaxPaymentSubjects = settings.maxPaymentSubjects
	e.cfg.MaxPaymentEventsPerSubject = settings.maxPaymentEventsPerSubject
	e.cfg.MaxPaymentsPerWindow = settings.maxPaymentsPerWindow
	e.cfg.PaymentLocationWarmupEvents = settings.paymentLocationWarmupEvents
	e.cfg.EnableExpiredKeySignal = settings.enableExpiredKeySignal
	e.cfg.EnableRevokedKeySignal = settings.enableRevokedKeySignal
	e.cfg.EnableExposedKeySignal = settings.enableExposedKeySignal
	e.cfg.EnablePromptOverrideSignals = settings.enablePromptOverrideSignals
	e.cfg.EnablePromptLengthSignal = settings.enablePromptLengthSignal
	e.cfg.MaxPromptBytes = settings.maxPromptBytes
	e.cfg.MaxPromptScanBytes = settings.maxPromptScanBytes
	e.cfg.EnableContextGuard = settings.enableContextGuard
	e.cfg.MaxContextMessages = settings.maxContextMessages
	e.cfg.MaxContextScanBytes = settings.maxContextScanBytes

	// Rebuild exemptions as keyed digests; neither the engine nor the reloader
	// retains clear-text IDs after this update.
	e.exemptions = make(map[string]struct{}, len(settings.exemptVirtualKeyIDs))
	for _, id := range settings.exemptVirtualKeyIDs {
		digest := e.profileDigest(id)
		e.exemptions[digest] = struct{}{}
		delete(e.profiles, digest)
		for _, cohort := range e.cohorts {
			delete(cohort.keys, digest)
		}
	}

	now := time.Now().UTC()
	e.pruneProfilesLocked(now)
	e.pruneCohortsLocked(now)
	e.prunePaymentProfilesLocked(now)
	for _, profile := range e.profiles {
		if len(profile.events) > e.cfg.MaxEventsPerProfile {
			profile.events = append([]event(nil), profile.events[len(profile.events)-e.cfg.MaxEventsPerProfile:]...)
		}
	}
	for len(e.profiles) > e.cfg.MaxProfiles {
		e.evictOldestLocked()
	}
	if !e.cfg.EnableCrossKeyCorrelation {
		clear(e.cohorts)
	} else {
		for _, cohort := range e.cohorts {
			for len(cohort.keys) > e.cfg.MaxDistinctKeysPerIdentity {
				var oldestID string
				var oldest time.Time
				for id, seen := range cohort.keys {
					if oldestID == "" || seen.Before(oldest) || (seen.Equal(oldest) && id < oldestID) {
						oldestID, oldest = id, seen
					}
				}
				delete(cohort.keys, oldestID)
			}
			cohort.alerted = false
		}
		for len(e.cohorts) > e.cfg.MaxProfiles {
			e.evictOldestCohortLocked()
		}
	}
	if !e.cfg.EnablePaymentVelocity && !e.cfg.EnablePaymentLocation {
		clear(e.payments)
	} else {
		for _, profile := range e.payments {
			if len(profile.events) > e.cfg.MaxPaymentEventsPerSubject {
				profile.events = append([]paymentObservation(nil), profile.events[len(profile.events)-e.cfg.MaxPaymentEventsPerSubject:]...)
			}
		}
		for len(e.payments) > e.cfg.MaxPaymentSubjects {
			e.evictOldestPaymentProfileLocked()
		}
	}
}

func (e *Engine) profileDigest(id string) string {
	return e.digest("virtual-key", id)
}

func (e *Engine) cohortDigest(id string) string {
	return e.digest("cohort", id)
}

func (e *Engine) digest(namespace, id string) string {
	mac := hmac.New(sha256.New, e.hashKey[:])
	_, _ = mac.Write([]byte(namespace))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(id))
	return hex.EncodeToString(mac.Sum(nil))
}

func (e *Engine) trackCohortKeyLocked(cohortID, keyID string, now time.Time) bool {
	c := e.cohorts[cohortID]
	if c != nil && now.Sub(c.lastSeen) > e.cfg.ProfileTTL {
		delete(e.cohorts, cohortID)
		c = nil
	}
	if c == nil {
		e.pruneCohortsLocked(now)
		if len(e.cohorts) >= e.cfg.MaxProfiles {
			e.evictOldestCohortLocked()
		}
		c = &cohortProfile{keys: make(map[string]time.Time), lastSeen: now}
		e.cohorts[cohortID] = c
	}
	for id, seen := range c.keys {
		if now.Sub(seen) > e.cfg.CorrelationWindow {
			delete(c.keys, id)
		}
	}
	newKey := false
	if _, exists := c.keys[keyID]; !exists && len(c.keys) <= e.cfg.MaxDistinctKeysPerIdentity {
		c.keys[keyID] = now
		newKey = true
	}
	c.lastSeen = now
	if len(c.keys) <= e.cfg.MaxDistinctKeysPerIdentity {
		c.alerted = false
		return false
	}
	if newKey && !c.alerted {
		c.alerted = true
		return true
	}
	return false
}

func (e *Engine) pruneCohortsLocked(now time.Time) int {
	removed := 0
	for id, c := range e.cohorts {
		if now.Sub(c.lastSeen) > e.cfg.ProfileTTL {
			delete(e.cohorts, id)
			removed++
			continue
		}
		for keyID, seen := range c.keys {
			if now.Sub(seen) > e.cfg.CorrelationWindow {
				delete(c.keys, keyID)
			}
		}
	}
	return removed
}

func (e *Engine) getOrCreatePaymentProfileLocked(id string, now time.Time) *paymentProfile {
	if p := e.payments[id]; p != nil {
		if now.Sub(p.lastSeen) > e.cfg.PaymentProfileTTL {
			delete(e.payments, id)
		} else {
			return p
		}
	}
	e.prunePaymentProfilesLocked(now)
	if len(e.payments) >= e.cfg.MaxPaymentSubjects {
		e.evictOldestPaymentProfileLocked()
	}
	p := &paymentProfile{
		events:   make([]paymentObservation, 0, min(e.cfg.MaxPaymentEventsPerSubject, 32)),
		lastSeen: now,
	}
	e.payments[id] = p
	return p
}

func (e *Engine) prunePaymentProfilesLocked(now time.Time) int {
	removed := 0
	for id, p := range e.payments {
		if now.Sub(p.lastSeen) > e.cfg.PaymentProfileTTL {
			delete(e.payments, id)
			removed++
			continue
		}
		e.prunePaymentEventsLocked(p, now)
	}
	return removed
}

func (e *Engine) prunePaymentEventsLocked(p *paymentProfile, now time.Time) {
	kept := make([]paymentObservation, 0, len(p.events))
	for _, event := range p.events {
		if event.at.After(now) || now.Sub(event.at) <= e.cfg.PaymentWindow {
			kept = append(kept, event)
		}
	}
	p.events = kept
}

func (e *Engine) evictOldestPaymentProfileLocked() {
	var oldestID string
	var oldest time.Time
	for id, p := range e.payments {
		if oldestID == "" || p.lastSeen.Before(oldest) {
			oldestID, oldest = id, p.lastSeen
		}
	}
	if oldestID != "" {
		delete(e.payments, oldestID)
	}
}

func (e *Engine) evictOldestCohortLocked() {
	var oldestID string
	var oldest time.Time
	for id, c := range e.cohorts {
		if oldestID == "" || c.lastSeen.Before(oldest) {
			oldestID, oldest = id, c.lastSeen
		}
	}
	if oldestID != "" {
		delete(e.cohorts, oldestID)
	}
}

func (e *Engine) getOrCreateProfileLocked(id string, now time.Time) *profile {
	if p := e.profiles[id]; p != nil {
		if now.Sub(p.lastSeen) > e.cfg.ProfileTTL {
			delete(e.profiles, id)
		} else {
			return p
		}
	}
	e.pruneProfilesLocked(now)
	if len(e.profiles) >= e.cfg.MaxProfiles {
		e.evictOldestLocked()
	}
	p := &profile{events: make([]event, 0, min(e.cfg.MaxEventsPerProfile, 32)), lastSeen: now}
	e.profiles[id] = p
	return p
}

func (e *Engine) pruneProfilesLocked(now time.Time) int {
	removed := 0
	for id, p := range e.profiles {
		if now.Sub(p.lastSeen) > e.cfg.ProfileTTL {
			delete(e.profiles, id)
			removed++
		}
	}
	return removed
}

func (e *Engine) evictOldestLocked() {
	var oldestID string
	var oldest time.Time
	for id, p := range e.profiles {
		if oldestID == "" || p.lastSeen.Before(oldest) {
			oldestID, oldest = id, p.lastSeen
		}
	}
	if oldestID != "" {
		delete(e.profiles, oldestID)
	}
}

func (e *Engine) pruneEventsLocked(p *profile, now time.Time) {
	first := 0
	for first < len(p.events) && now.Sub(p.events[first].at) > e.cfg.Window {
		first++
	}
	if first > 0 {
		p.events = append([]event(nil), p.events[first:]...)
	}
}

func (e *Engine) makeFindingLocked(now time.Time, signals []Signal, warmup bool) Finding {
	return e.makeFindingAtThresholdLocked(now, signals, warmup, e.cfg.AlertThreshold)
}

func (e *Engine) makeFindingAtThresholdLocked(now time.Time, signals []Signal, warmup bool, threshold int) Finding {
	sort.Slice(signals, func(i, j int) bool { return signals[i].Name < signals[j].Name })
	score := 0
	for _, signal := range signals {
		score += signal.Weight
	}
	if score > 100 {
		score = 100
	}
	finding := Finding{
		Timestamp: now,
		Score:     score,
		Threshold: threshold,
		Alert:     score >= threshold,
		Warmup:    warmup,
		Signals:   append([]Signal(nil), signals...),
	}
	if len(signals) > 0 {
		e.metrics.Findings++
	}
	if finding.Alert {
		e.metrics.Alerts++
	}
	return finding
}

func (e *Engine) emit(sink func(Finding), finding Finding, aiReviewAllowed bool) {
	if sink != nil && len(finding.Signals) > 0 {
		e.enqueueFindingCallback(finding)
	}
	if aiReviewAllowed {
		e.enqueueAIReview(finding)
	}
}

func (e *Engine) emptyFinding(now time.Time, warmup bool) Finding {
	e.mu.Lock()
	threshold := e.cfg.AlertThreshold
	e.mu.Unlock()
	return Finding{Timestamp: now.UTC(), Threshold: threshold, Warmup: warmup}
}

func normalizeCountryCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 2 {
		return ""
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return ""
		}
	}
	return value
}

func hasPaymentLocation(events []paymentObservation, countryDigest string) bool {
	for _, event := range events {
		if event.countryDigest == countryDigest {
			return true
		}
	}
	return false
}

func countPaymentLocations(events []paymentObservation) int {
	count := 0
	for _, event := range events {
		if event.countryDigest != "" {
			count++
		}
	}
	return count
}

func safeLabel(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return value[:128]
	}
	return value
}

func hasLabel(events []event, value string, provider bool) bool {
	if value == "" {
		return false
	}
	for _, item := range events {
		candidate := item.model
		if provider {
			candidate = item.provider
		}
		if candidate == value {
			return true
		}
	}
	return false
}

func isTimingAnomaly(events []event, now time.Time, window time.Duration) bool {
	times := make([]time.Time, 0, len(events))
	for _, item := range events {
		if !item.at.After(now) && now.Sub(item.at) <= window {
			times = append(times, item.at)
		}
	}
	if len(times) < 4 {
		return false
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	gaps := make([]time.Duration, 0, len(times)-1)
	for i := 1; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}
	if len(gaps) < 3 {
		return false
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
	median := gaps[len(gaps)/2]
	currentGap := now.Sub(times[len(times)-1])
	return currentGap > 0 && currentGap < median/4 && currentGap < window/10
}

func tokenValues(events []event, sequence uint64, input bool) []int64 {
	values := make([]int64, 0, len(events))
	for _, item := range events {
		if item.sequence == sequence || !item.complete {
			continue
		}
		value := item.output
		if input {
			value = item.input
		}
		if value >= 0 {
			values = append(values, value)
		}
	}
	return values
}

func isTokenSpike(baseline []int64, current int64, absoluteFloor int64) bool {
	if len(baseline) < 2 || current <= 0 {
		return false
	}
	var sum int64
	for _, value := range baseline {
		sum += value
	}
	mean := float64(sum) / float64(len(baseline))
	limit := math.Max(mean*3, mean+float64(absoluteFloor))
	return float64(current) > limit
}

func isRepeatedFailure(events []event, sequence uint64, currentFailed bool, now time.Time, window time.Duration) bool {
	if !currentFailed {
		return false
	}
	recent := make([]event, 0, 5)
	for _, item := range events {
		if item.sequence == sequence || !item.complete || item.at.After(now) || now.Sub(item.at) > window {
			continue
		}
		recent = append(recent, item)
	}
	current := event{sequence: sequence, at: now, complete: true, failed: true}
	recent = append(recent, current)
	sort.Slice(recent, func(i, j int) bool {
		if recent[i].at.Equal(recent[j].at) {
			return recent[i].sequence < recent[j].sequence
		}
		return recent[i].at.Before(recent[j].at)
	})
	if len(recent) > 5 {
		recent = recent[len(recent)-5:]
	}
	recentIDs := make(map[uint64]struct{}, len(recent))
	failures := 0
	for _, item := range recent {
		recentIDs[item.sequence] = struct{}{}
		if item.failed {
			failures++
		}
	}
	if len(recent) < 3 || failures < 3 || failures*5 < len(recent)*3 {
		return false
	}
	var baselineCompleted, baselineFailures int
	for _, item := range events {
		if item.sequence == sequence || !item.complete {
			continue
		}
		if _, inRecent := recentIDs[item.sequence]; inRecent {
			continue
		}
		baselineCompleted++
		if item.failed {
			baselineFailures++
		}
	}
	return baselineCompleted == 0 || baselineFailures*2 <= baselineCompleted
}

func completedCount(events []event, sequence uint64) int {
	count := 0
	for _, item := range events {
		if item.sequence != sequence && item.complete {
			count++
		}
	}
	return count
}

func eventIndex(events []event, sequence uint64) int {
	for i := range events {
		if events[i].sequence == sequence {
			return i
		}
	}
	return -1
}

func nonnegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	if value > 1_000_000_000 {
		return 1_000_000_000
	}
	return value
}
