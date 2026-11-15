// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

// Package httpapi exposes FraudGuard's observe-only scorer to trusted host
// applications. It accepts request metadata only; prompt and payload fields
// are deliberately not part of the request schema.
package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	fraudguard "github.com/maximhq/bifrost/plugins/fraudguard"
)

const (
	ScorePath                = "/api/v1/score"
	HealthPath               = "/healthz"
	maxRequestBytes          = 4096
	maxAPIKeys               = 1000
	defaultRequestsPerMinute = 120
	idempotencyTTL           = 10 * time.Minute
	maxIdempotencyEntries    = 20_000
)

var (
	identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	labelPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$`)
)

// Config contains only non-secret service settings. API key material is
// configured as SHA-256 hashes mapped to opaque tenant references.
type Config struct {
	// KeyHashes maps a lowercase SHA-256 hex digest of an API key to a stable,
	// opaque tenant reference. Raw API keys are never stored by the service.
	KeyHashes map[string]string
	// RequestsPerMinute is a fixed-window per-tenant safety limit.
	RequestsPerMinute int
}

type credential struct {
	digest    [sha256.Size]byte
	tenantRef string
}

// ScoreRequest is a completed, metadata-only event from a trusted host.
// ProfileRef must be a stable pseudonymous identifier, not a raw user or key.
type ScoreRequest struct {
	EventID      string    `json:"event_id"`
	ProfileRef   string    `json:"profile_ref"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Timestamp    time.Time `json:"timestamp"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	Failed       bool      `json:"failed"`
}

// ScoreResponse is an advisory only. Enforcement is never performed here.
type ScoreResponse struct {
	SchemaVersion string              `json:"schema_version"`
	EventID       string              `json:"event_id"`
	RiskScore     int                 `json:"risk_score"`
	Threshold     int                 `json:"threshold"`
	Alert         bool                `json:"alert"`
	Warmup        bool                `json:"warmup"`
	Signals       []fraudguard.Signal `json:"signals"`
	Enforcement   string              `json:"enforcement"`
}

type Handler struct {
	engine      *fraudguard.Engine
	credentials []credential
	rateLimit   int
	rateMu      sync.Mutex
	rateWindows map[string]rateWindow
	idempotency *idempotencyStore
}

type rateWindow struct {
	start time.Time
	count int
}

// NewHandler validates configuration and returns the public score and health
// routes. Callers should place it behind HTTPS and network-level protection.
func NewHandler(engine *fraudguard.Engine, config Config) (http.Handler, error) {
	if engine == nil {
		return nil, errors.New("fraudguard http: engine is required")
	}
	if len(config.KeyHashes) == 0 || len(config.KeyHashes) > maxAPIKeys {
		return nil, fmt.Errorf("fraudguard http: configure between 1 and %d API key hashes", maxAPIKeys)
	}
	if config.RequestsPerMinute == 0 {
		config.RequestsPerMinute = defaultRequestsPerMinute
	}
	if config.RequestsPerMinute < 1 || config.RequestsPerMinute > 100_000 {
		return nil, errors.New("fraudguard http: requests per minute must be between 1 and 100000")
	}

	handler := &Handler{
		engine:      engine,
		rateLimit:   config.RequestsPerMinute,
		rateWindows: make(map[string]rateWindow),
		idempotency: newIdempotencyStore(maxIdempotencyEntries, idempotencyTTL),
	}
	for keyHash, tenantRef := range config.KeyHashes {
		decoded, err := hex.DecodeString(keyHash)
		if err != nil || len(decoded) != sha256.Size || strings.ToLower(keyHash) != keyHash {
			return nil, errors.New("fraudguard http: API key hash must be lowercase SHA-256 hex")
		}
		if !identifierPattern.MatchString(tenantRef) {
			return nil, errors.New("fraudguard http: tenant reference must be an opaque 1-128 character identifier")
		}
		var digest [sha256.Size]byte
		copy(digest[:], decoded)
		handler.credentials = append(handler.credentials, credential{digest: digest, tenantRef: tenantRef})
	}
	sort.Slice(handler.credentials, func(i, j int) bool {
		return handler.credentials[i].tenantRef < handler.credentials[j].tenantRef
	})
	return handler, nil
}

// ParseKeyHashMappings parses FRAUDGUARD_API_KEY_HASHES in the form
// "sha256hex=tenant-ref;sha256hex=tenant-ref". It never accepts raw API keys.
func ParseKeyHashMappings(value string) (map[string]string, error) {
	result := make(map[string]string)
	for _, item := range strings.Split(value, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		hash, tenantRef, ok := strings.Cut(item, "=")
		hash = strings.TrimSpace(hash)
		tenantRef = strings.TrimSpace(tenantRef)
		if !ok || hash == "" || tenantRef == "" {
			return nil, errors.New("fraudguard http: invalid API key mapping")
		}
		if _, exists := result[hash]; exists {
			return nil, errors.New("fraudguard http: duplicate API key hash")
		}
		result[hash] = tenantRef
	}
	if len(result) == 0 {
		return nil, errors.New("fraudguard http: no API key mappings configured")
	}
	return result, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	switch r.URL.Path {
	case HealthPath:
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case ScorePath:
		h.score(w, r)
	default:
		writeError(w, http.StatusNotFound, "not_found")
	}
}

func (h *Handler) score(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	tenantRef, ok := h.authenticate(r.Header.Get("Authorization"))
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="fraudguard"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.allow(tenantRef, time.Now()) {
		writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded")
		return
	}

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "application_json_required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input ScoreRequest
	if err := decoder.Decode(&input); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "one_json_object_required")
		return
	}
	if err := validateScoreRequest(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	cacheKey := idempotencyKey(tenantRef, input.EventID)
	response, err := h.idempotency.Do(r.Context(), cacheKey, func() (ScoreResponse, error) {
		return h.evaluate(tenantRef, input), nil
	})
	if err != nil {
		if errors.Is(err, errIdempotencyCapacity) {
			writeError(w, http.StatusServiceUnavailable, "scoring_temporarily_unavailable")
			return
		}
		writeError(w, http.StatusRequestTimeout, "request_cancelled")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) authenticate(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) || len(header) <= len(prefix) || len(header) > 512 {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return "", false
	}
	presented := sha256.Sum256([]byte(token))
	tenantRef := ""
	matched := 0
	for _, item := range h.credentials {
		equal := subtle.ConstantTimeCompare(presented[:], item.digest[:])
		if equal == 1 {
			tenantRef = item.tenantRef
		}
		matched |= equal
	}
	return tenantRef, matched == 1
}

func (h *Handler) allow(tenantRef string, now time.Time) bool {
	h.rateMu.Lock()
	defer h.rateMu.Unlock()

	window := h.rateWindows[tenantRef]
	if window.start.IsZero() || now.Sub(window.start) >= time.Minute || now.Before(window.start) {
		window = rateWindow{start: now}
	}
	if window.count >= h.rateLimit {
		h.rateWindows[tenantRef] = window
		return false
	}
	window.count++
	h.rateWindows[tenantRef] = window
	return true
}

func (h *Handler) evaluate(tenantRef string, input ScoreRequest) ScoreResponse {
	// The engine HMACs this scoped pseudonymous reference immediately. Neither
	// component is returned, logged, or retained as plaintext by the engine.
	profileID := tenantRef + ":" + input.ProfileRef
	handle, start := h.engine.Start(fraudguard.Request{
		VirtualKeyID: profileID,
		Provider:     input.Provider,
		Model:        input.Model,
		Timestamp:    input.Timestamp,
	})
	finish := h.engine.Complete(handle, fraudguard.Completion{
		InputTokens:  input.InputTokens,
		OutputTokens: input.OutputTokens,
		Failed:       input.Failed,
		Timestamp:    input.Timestamp,
	})
	finding := combineFindings(start.Finding, finish.Finding, input.Timestamp)
	return ScoreResponse{
		SchemaVersion: "fraudguard.score.v1",
		EventID:       input.EventID,
		RiskScore:     finding.Score,
		Threshold:     finding.Threshold,
		Alert:         finding.Alert,
		Warmup:        finding.Warmup,
		Signals:       finding.Signals,
		Enforcement:   "observe_only",
	}
}

func combineFindings(start, finish fraudguard.Finding, at time.Time) fraudguard.Finding {
	byName := make(map[string]fraudguard.Signal, len(start.Signals)+len(finish.Signals))
	for _, signal := range append(append([]fraudguard.Signal(nil), start.Signals...), finish.Signals...) {
		if current, ok := byName[signal.Name]; !ok || signal.Weight > current.Weight {
			byName[signal.Name] = signal
		}
	}
	signals := make([]fraudguard.Signal, 0, len(byName))
	score := 0
	for _, signal := range byName {
		signals = append(signals, signal)
		score += signal.Weight
	}
	sort.Slice(signals, func(i, j int) bool { return signals[i].Name < signals[j].Name })
	if score > 100 {
		score = 100
	}
	threshold := start.Threshold
	if threshold < 1 || threshold > 100 {
		threshold = finish.Threshold
	}
	return fraudguard.Finding{
		Timestamp: at.UTC(),
		Score:     score,
		Threshold: threshold,
		Alert:     score >= threshold,
		Warmup:    start.Warmup || finish.Warmup,
		Signals:   signals,
	}
}

func validateScoreRequest(input ScoreRequest) error {
	if !identifierPattern.MatchString(input.EventID) {
		return errors.New("invalid_event_id")
	}
	if !identifierPattern.MatchString(input.ProfileRef) {
		return errors.New("invalid_profile_ref")
	}
	if !labelPattern.MatchString(input.Provider) || !labelPattern.MatchString(input.Model) {
		return errors.New("invalid_provider_or_model")
	}
	if input.Timestamp.IsZero() {
		return errors.New("timestamp_required")
	}
	now := time.Now()
	if input.Timestamp.Before(now.Add(-24*time.Hour)) || input.Timestamp.After(now.Add(2*time.Minute)) {
		return errors.New("timestamp_out_of_range")
	}
	if input.InputTokens < 0 || input.OutputTokens < 0 ||
		input.InputTokens > 100_000_000 || input.OutputTokens > 100_000_000 {
		return errors.New("token_count_out_of_range")
	}
	return nil
}

func idempotencyKey(tenantRef, eventID string) string {
	digest := sha256.Sum256([]byte(tenantRef + "\x00" + eventID))
	return hex.EncodeToString(digest[:])
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_json")
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

var errIdempotencyCapacity = errors.New("fraudguard http: idempotency store is full")

type idempotencyEntry struct {
	done      chan struct{}
	response  ScoreResponse
	err       error
	expiresAt time.Time
	complete  bool
}

type idempotencyStore struct {
	mu      sync.Mutex
	entries map[string]*idempotencyEntry
	max     int
	ttl     time.Duration
}

func newIdempotencyStore(max int, ttl time.Duration) *idempotencyStore {
	return &idempotencyStore{entries: make(map[string]*idempotencyEntry), max: max, ttl: ttl}
}

func (s *idempotencyStore) Do(
	ctx context.Context,
	key string,
	evaluate func() (ScoreResponse, error),
) (ScoreResponse, error) {
	for {
		s.mu.Lock()
		s.pruneLocked(time.Now())
		if existing := s.entries[key]; existing != nil {
			done := existing.done
			if existing.complete {
				response, err := existing.response, existing.err
				s.mu.Unlock()
				return response, err
			}
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return ScoreResponse{}, ctx.Err()
			case <-done:
				continue
			}
		}
		if len(s.entries) >= s.max && !s.evictOldestLocked() {
			s.mu.Unlock()
			return ScoreResponse{}, errIdempotencyCapacity
		}
		entry := &idempotencyEntry{done: make(chan struct{})}
		s.entries[key] = entry
		s.mu.Unlock()

		response, err := evaluate()
		s.mu.Lock()
		entry.response = response
		entry.err = err
		entry.complete = true
		entry.expiresAt = time.Now().Add(s.ttl)
		close(entry.done)
		s.mu.Unlock()
		return response, err
	}
}

func (s *idempotencyStore) pruneLocked(now time.Time) {
	for key, entry := range s.entries {
		if entry.complete && !entry.expiresAt.After(now) {
			delete(s.entries, key)
		}
	}
}

func (s *idempotencyStore) evictOldestLocked() bool {
	var oldestKey string
	var oldest time.Time
	for key, entry := range s.entries {
		if entry.complete && (oldestKey == "" || entry.expiresAt.Before(oldest)) {
			oldestKey, oldest = key, entry.expiresAt
		}
	}
	if oldestKey == "" {
		return false
	}
	delete(s.entries, oldestKey)
	return true
}
