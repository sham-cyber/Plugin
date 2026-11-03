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
	// AlertThreshold is inclusive; findings below it remain observable but are not alerts.
	AlertThreshold int
	// ExemptVirtualKeyIDs contains resolved internal IDs, never credential values.
	// IDs are immediately HMACed and are not stored in clear text.
	ExemptVirtualKeyIDs []string
	// OnFinding receives structured, payload-free findings. It is invoked outside the lock.
	OnFinding func(Finding)
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
	Provider     string
	Model        string
	Timestamp    time.Time
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
	profileID string
	sequence  uint64
}

// Assessment is the current score and its explanation.
type Assessment struct {
	Finding Finding
}

// Metrics is a privacy-preserving aggregate snapshot.
type Metrics struct {
	Profiles       int    `json:"profiles"`
	Requests       uint64 `json:"requests"`
	Findings       uint64 `json:"findings"`
	Alerts         uint64 `json:"alerts"`
	ExemptRequests uint64 `json:"exempt_requests"`
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

// Engine owns the thread-safe profile store and deterministic scoring logic.
type Engine struct {
	mu         sync.Mutex
	cfg        Config
	hashKey    [32]byte
	exemptions map[string]struct{}
	profiles   map[string]*profile
	nextID     uint64
	metrics    Metrics
}

// NewEngine creates an in-memory detector. A random process-local HMAC key ensures
// profile map keys cannot be reversed or correlated across restarts.
func NewEngine(config Config) (*Engine, error) {
	config = withDefaults(config)
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, errors.New("fraudguard: unable to initialize private profile hashing")
	}
	engine := &Engine{
		cfg:        config,
		hashKey:    secret,
		exemptions: make(map[string]struct{}, len(config.ExemptVirtualKeyIDs)),
		profiles:   make(map[string]*profile),
	}
	for _, id := range config.ExemptVirtualKeyIDs {
		if strings.TrimSpace(id) != "" {
			engine.exemptions[engine.profileDigest(id)] = struct{}{}
		}
	}
	// Keep only digests; the caller-provided identifiers must not remain in config.
	engine.cfg.ExemptVirtualKeyIDs = nil
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
	if config.AlertThreshold <= 0 {
		config.AlertThreshold = 60
	}
	if config.AlertThreshold > 100 {
		config.AlertThreshold = 100
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
	if _, exempt := e.exemptions[id]; exempt {
		e.metrics.ExemptRequests++
		e.mu.Unlock()
		return Handle{}, Assessment{Finding: e.emptyFinding(now, false)}
	}
	p := e.getOrCreateProfileLocked(id, now)
	e.pruneEventsLocked(p, now)
	warmup := len(p.events) < e.cfg.WarmupRequests
	signals := make([]Signal, 0, 3)
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
	p.events = append(p.events, event{
		sequence: sequence,
		at:       now,
		provider: provider,
		model:    model,
	})
	if len(p.events) > e.cfg.MaxEventsPerProfile {
		p.events = append([]event(nil), p.events[len(p.events)-e.cfg.MaxEventsPerProfile:]...)
	}
	p.lastSeen = now
	e.metrics.Requests++
	finding := e.makeFindingLocked(now, signals, warmup)
	sink := e.cfg.OnFinding
	e.mu.Unlock()
	e.emit(sink, finding)

	return Handle{profileID: id, sequence: sequence}, Assessment{Finding: finding}
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
	finding := e.makeFindingLocked(now, signals, warmup)
	sink := e.cfg.OnFinding
	e.mu.Unlock()
	e.emit(sink, finding)
	return Assessment{Finding: finding}
}

// Prune expires profiles that have not been observed for ProfileTTL.
func (e *Engine) Prune(now time.Time) int {
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pruneProfilesLocked(now)
}

// Metrics returns aggregate counters without exposing individual profile identifiers.
func (e *Engine) Metrics() Metrics {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.metrics
	out.Profiles = len(e.profiles)
	return out
}

func (e *Engine) profileDigest(id string) string {
	mac := hmac.New(sha256.New, e.hashKey[:])
	_, _ = mac.Write([]byte(id))
	return hex.EncodeToString(mac.Sum(nil))
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
		Threshold: e.cfg.AlertThreshold,
		Alert:     score >= e.cfg.AlertThreshold,
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

func (e *Engine) emit(sink func(Finding), finding Finding) {
	if sink != nil && len(finding.Signals) > 0 {
		sink(finding)
	}
}

func (e *Engine) emptyFinding(now time.Time, warmup bool) Finding {
	return Finding{Timestamp: now.UTC(), Threshold: e.cfg.AlertThreshold, Warmup: warmup}
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
