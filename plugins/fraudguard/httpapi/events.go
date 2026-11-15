// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"sync"
	"time"

	fraudguard "github.com/maximhq/bifrost/plugins/fraudguard"
)

const (
	OperationModelSummary = "model_summary"
	OperationTavilySearch = "tavily_search"

	EventStart             = "start"
	EventCompletion        = "completion"
	EventFailure           = "failure"
	EventCancellation      = "cancellation"
	EventCitationGuardrail = "citation_guardrail"

	TokenCountProviderReported = "provider-reported"
	TokenCountEstimated        = "estimated"
	TokenCountUnavailable      = "unavailable"

	eventSchemaVersion = "fraudguard.event.v2"
	maxEventDurationMS = int64(10 * 60 * 1000)
	maxTavilyResults   = int64(1000)
)

// EventRequest is the strict, metadata-only v2 host event contract.
// ProfileRef must be opaque and pseudonymous; content and credentials are not fields.
type EventRequest struct {
	EventID           string    `json:"event_id"`
	AttemptID         string    `json:"attempt_id"`
	ProfileRef        string    `json:"profile_ref"`
	Operation         string    `json:"operation"`
	EventType         string    `json:"event_type"`
	Timestamp         time.Time `json:"timestamp"`
	Status            string    `json:"status"`
	Provider          string    `json:"provider,omitempty"`
	Model             string    `json:"model,omitempty"`
	InputTokens       *int64    `json:"input_tokens,omitempty"`
	OutputTokens      *int64    `json:"output_tokens,omitempty"`
	TokenCountSource  string    `json:"token_count_source,omitempty"`
	DurationMS        *int64    `json:"duration_ms,omitempty"`
	ResultCount       *int64    `json:"result_count,omitempty"`
	ValidationOutcome string    `json:"validation_outcome,omitempty"`
}

// EventResponse acknowledges an event. Score is present only for terminal
// model-summary and Tavily-search events; lifecycle and guardrail events are
// acknowledged without treating them as additional requests.
type EventResponse struct {
	SchemaVersion string         `json:"schema_version"`
	EventID       string         `json:"event_id"`
	AttemptID     string         `json:"attempt_id"`
	Accepted      bool           `json:"accepted"`
	Deduplicated  bool           `json:"deduplicated,omitempty"`
	Operation     string         `json:"operation"`
	EventType     string         `json:"event_type"`
	Status        string         `json:"status"`
	Scored        bool           `json:"scored"`
	Score         *ScoreResponse `json:"score,omitempty"`
	Enforcement   string         `json:"enforcement"`
}

func (h *Handler) eventsEndpoint(w http.ResponseWriter, r *http.Request) {
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
	var input EventRequest
	if err := decoder.Decode(&input); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "one_json_object_required")
		return
	}
	if err := validateEventRequest(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	canonical, err := json.Marshal(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	fingerprint := sha256.Sum256(canonical)
	cacheKey := idempotencyKey(tenantRef, input.EventID)
	response, duplicate, err := h.events.Do(r.Context(), cacheKey, fingerprint, func() (EventResponse, error) {
		return h.evaluateEvent(tenantRef, input), nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errEventIdempotencyConflict):
			writeError(w, http.StatusConflict, "event_id_conflict")
		case errors.Is(err, errIdempotencyCapacity):
			writeError(w, http.StatusServiceUnavailable, "event_ingestion_temporarily_unavailable")
		default:
			writeError(w, http.StatusRequestTimeout, "request_cancelled")
		}
		return
	}
	response.Deduplicated = duplicate
	writeJSON(w, http.StatusAccepted, response)
}

func (h *Handler) evaluateEvent(tenantRef string, input EventRequest) EventResponse {
	response := EventResponse{
		SchemaVersion: eventSchemaVersion,
		EventID:       input.EventID,
		AttemptID:     input.AttemptID,
		Accepted:      true,
		Operation:     input.Operation,
		EventType:     input.EventType,
		Status:        input.Status,
		Enforcement:   "observe_only",
	}
	if input.EventType != EventCompletion && input.EventType != EventFailure && input.EventType != EventCancellation {
		return response
	}

	provider := input.Provider
	model := input.Model
	var inputTokens, outputTokens int64
	if input.Operation == OperationTavilySearch {
		// Searches participate in the same profile's provider/frequency/error
		// baseline without pretending that a search provider is an LLM model.
		model = ""
	} else {
		inputTokens = tokenValue(input.InputTokens)
		outputTokens = tokenValue(input.OutputTokens)
	}
	handle, start := h.engine.Start(fraudguard.Request{
		VirtualKeyID: tenantRef + ":" + input.ProfileRef,
		Provider:     provider,
		Model:        model,
		Timestamp:    input.Timestamp,
	})
	finish := h.engine.Complete(handle, fraudguard.Completion{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Failed:       input.EventType == EventFailure || input.EventType == EventCancellation,
		Timestamp:    input.Timestamp,
	})
	finding := combineFindings(start.Finding, finish.Finding, input.Timestamp)
	response.Scored = true
	response.Score = &ScoreResponse{
		SchemaVersion: "fraudguard.score.v1",
		EventID:       input.EventID,
		RiskScore:     finding.Score,
		Threshold:     finding.Threshold,
		Alert:         finding.Alert,
		Warmup:        finding.Warmup,
		Signals:       finding.Signals,
		Enforcement:   "observe_only",
	}
	return response
}

func tokenValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func validateEventRequest(input EventRequest) error {
	if !identifierPattern.MatchString(input.EventID) {
		return errors.New("invalid_event_id")
	}
	if !identifierPattern.MatchString(input.AttemptID) {
		return errors.New("invalid_attempt_id")
	}
	if !identifierPattern.MatchString(input.ProfileRef) {
		return errors.New("invalid_profile_ref")
	}
	if input.Operation != OperationModelSummary && input.Operation != OperationTavilySearch {
		return errors.New("invalid_operation")
	}
	if input.Timestamp.IsZero() {
		return errors.New("timestamp_required")
	}
	now := time.Now()
	if input.Timestamp.Before(now.Add(-24*time.Hour)) || input.Timestamp.After(now.Add(2*time.Minute)) {
		return errors.New("timestamp_out_of_range")
	}
	if err := validateEventState(input); err != nil {
		return err
	}
	if input.DurationMS != nil && (*input.DurationMS < 0 || *input.DurationMS > maxEventDurationMS) {
		return errors.New("duration_out_of_range")
	}
	if input.ResultCount != nil && (*input.ResultCount < 0 || *input.ResultCount > maxTavilyResults) {
		return errors.New("result_count_out_of_range")
	}

	switch input.Operation {
	case OperationModelSummary:
		if !labelPattern.MatchString(input.Provider) || !labelPattern.MatchString(input.Model) {
			return errors.New("invalid_provider_or_model")
		}
		if input.ResultCount != nil {
			return errors.New("result_count_not_allowed")
		}
		if isTerminalEvent(input.EventType) {
			if input.DurationMS == nil {
				return errors.New("duration_required")
			}
			if err := validateTokenCounts(input); err != nil {
				return err
			}
		} else if input.InputTokens != nil || input.OutputTokens != nil ||
			(input.TokenCountSource != "" && input.TokenCountSource != TokenCountUnavailable) {
			return errors.New("token_counts_not_allowed_for_event")
		}
	case OperationTavilySearch:
		if input.Provider != "tavily" || input.Model != "" {
			return errors.New("invalid_tavily_provider_or_model")
		}
		if input.InputTokens != nil || input.OutputTokens != nil ||
			(input.TokenCountSource != "" && input.TokenCountSource != TokenCountUnavailable) {
			return errors.New("token_counts_not_allowed_for_tavily")
		}
		if input.EventType == EventCompletion {
			if input.DurationMS == nil {
				return errors.New("duration_required")
			}
			if input.ResultCount == nil {
				return errors.New("result_count_required")
			}
		}
		if (input.EventType == EventFailure || input.EventType == EventCancellation) && input.DurationMS == nil {
			return errors.New("duration_required")
		}
	}
	return nil
}

func validateEventState(input EventRequest) error {
	switch input.EventType {
	case EventStart:
		if input.Status != "started" || input.ValidationOutcome != "" {
			return errors.New("invalid_start_status")
		}
	case EventCompletion:
		if input.Status != "completed" || input.ValidationOutcome != "" {
			return errors.New("invalid_completion_status")
		}
	case EventFailure:
		if input.Status != "failed" || input.ValidationOutcome != "" {
			return errors.New("invalid_failure_status")
		}
	case EventCancellation:
		if input.Status != "cancelled" || input.ValidationOutcome != "" {
			return errors.New("invalid_cancellation_status")
		}
	case EventCitationGuardrail:
		if input.ValidationOutcome != "passed" && input.ValidationOutcome != "failed" && input.ValidationOutcome != "not_run" {
			return errors.New("invalid_validation_outcome")
		}
		if input.Status != input.ValidationOutcome {
			return errors.New("status_must_match_validation_outcome")
		}
	default:
		return errors.New("invalid_event_type")
	}
	return nil
}

func validateTokenCounts(input EventRequest) error {
	switch input.TokenCountSource {
	case TokenCountProviderReported, TokenCountEstimated:
		if input.InputTokens == nil || input.OutputTokens == nil {
			return errors.New("token_counts_required")
		}
		if *input.InputTokens < 0 || *input.OutputTokens < 0 ||
			*input.InputTokens > 100_000_000 || *input.OutputTokens > 100_000_000 {
			return errors.New("token_count_out_of_range")
		}
	case TokenCountUnavailable:
		if input.InputTokens != nil || input.OutputTokens != nil {
			return errors.New("unavailable_token_counts_must_be_omitted")
		}
	default:
		return errors.New("invalid_token_count_source")
	}
	return nil
}

func isTerminalEvent(eventType string) bool {
	return eventType == EventCompletion || eventType == EventFailure || eventType == EventCancellation
}

var (
	errEventIdempotencyConflict = errors.New("fraudguard http: event ID reused with different content")
)

type eventIdempotencyEntry struct {
	done        chan struct{}
	fingerprint [sha256.Size]byte
	response    EventResponse
	err         error
	expiresAt   time.Time
	complete    bool
}

type eventIdempotencyStore struct {
	mu      sync.Mutex
	entries map[string]*eventIdempotencyEntry
	max     int
	ttl     time.Duration
}

func newEventIdempotencyStore(max int, ttl time.Duration) *eventIdempotencyStore {
	return &eventIdempotencyStore{entries: make(map[string]*eventIdempotencyEntry), max: max, ttl: ttl}
}

func (s *eventIdempotencyStore) Do(
	ctx context.Context,
	key string,
	fingerprint [sha256.Size]byte,
	evaluate func() (EventResponse, error),
) (EventResponse, bool, error) {
	for {
		s.mu.Lock()
		s.pruneLocked(time.Now())
		if existing := s.entries[key]; existing != nil {
			if existing.fingerprint != fingerprint {
				s.mu.Unlock()
				return EventResponse{}, false, errEventIdempotencyConflict
			}
			if existing.complete {
				response, err := existing.response, existing.err
				s.mu.Unlock()
				response.Deduplicated = true
				return response, true, err
			}
			done := existing.done
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return EventResponse{}, false, ctx.Err()
			case <-done:
				continue
			}
		}
		if len(s.entries) >= s.max && !s.evictOldestLocked() {
			s.mu.Unlock()
			return EventResponse{}, false, errIdempotencyCapacity
		}
		entry := &eventIdempotencyEntry{done: make(chan struct{}), fingerprint: fingerprint}
		s.entries[key] = entry
		s.mu.Unlock()

		response, err := evaluate()
		s.mu.Lock()
		if err != nil {
			delete(s.entries, key)
			entry.err = err
			entry.complete = true
			close(entry.done)
			s.mu.Unlock()
			return EventResponse{}, false, err
		}
		entry.response = response
		entry.complete = true
		entry.expiresAt = time.Now().Add(s.ttl)
		close(entry.done)
		s.mu.Unlock()
		return response, false, nil
	}
}

func (s *eventIdempotencyStore) pruneLocked(now time.Time) {
	for key, entry := range s.entries {
		if entry.complete && !entry.expiresAt.After(now) {
			delete(s.entries, key)
		}
	}
}

func (s *eventIdempotencyStore) evictOldestLocked() bool {
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
