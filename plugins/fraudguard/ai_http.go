// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	envAIReviewEndpoint = "FRAUDGUARD_AI_REVIEW_ENDPOINT"
	envAIReviewModel    = "FRAUDGUARD_AI_REVIEW_MODEL"
	envAIReviewAPIKey   = "FRAUDGUARD_AI_REVIEW_API_KEY"
	envAIReviewKeyID    = "FRAUDGUARD_AI_REVIEW_VIRTUAL_KEY_ID"

	maxAIReviewResponseBytes = 64 * 1024
	maxAIReviewRequestBytes  = 8 * 1024
	maxAIReviewSignals       = 64
)

const aiReviewSystemMessage = `You are an advisory reviewer for an AI-gateway behavior monitor.
Review only the supplied numeric score and fixed signal names. Do not claim that fraud is proven.
Return exactly one JSON object. The conclusion must be exactly one of: review_suggested, no_added_concern, insufficient_data.
Example: {"conclusion":"review_suggested","confidence":50,"explanation":"The combined signals merit operator review."}
Confidence is a descriptive 0-100 value, not a calibrated probability. Do not recommend blocking or changing a request.`

var allowedAIReviewSignals = map[string]struct{}{
	"configuration_bounds":          {},
	"context_data_exfiltration":     {},
	"context_progressive_jailbreak": {},
	"context_schema_manipulation":   {},
	"custom_prompt_pattern":         {},
	"debug_token_auth":              {},
	"identity_source":               {},
	"input_volume_spike":            {},
	"jwt_verification":              {},
	"key_expired":                   {},
	"key_exposed":                   {},
	"key_revoked":                   {},
	"key_rotation":                  {},
	"new_model":                     {},
	"new_provider":                  {},
	"output_volume_spike":           {},
	"payment_new_location":          {},
	"payment_velocity":              {},
	"prompt_inspection":             {},
	"prompt_override_phrase":        {},
	"prompt_size_spike":             {},
	"rapid_sequence":                {},
	"repeated_errors":               {},
	"request_handling":              {},
	"secret_values":                 {},
	"trusted_threat_indicator":      {},
}

// BifrostHTTPAIAnalyzerConfig configures an optional call to the host's
// OpenAI-compatible Bifrost endpoint. APIKey is held only in memory and is
// never included in errors or logs.
type BifrostHTTPAIAnalyzerConfig struct {
	Endpoint string
	Model    string
	APIKey   string
	Timeout  time.Duration
}

// BifrostHTTPAIAnalyzer asks the host's Bifrost model for a separate,
// asynchronous advisory about an already-computed deterministic finding.
type BifrostHTTPAIAnalyzer struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

// NewBifrostHTTPAIAnalyzer creates an adapter for a trusted Bifrost chat
// completions endpoint. Cleartext HTTP is accepted only for loopback hosts.
func NewBifrostHTTPAIAnalyzer(config BifrostHTTPAIAnalyzerConfig) (*BifrostHTTPAIAnalyzer, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	model := strings.TrimSpace(config.Model)
	apiKey := strings.TrimSpace(config.APIKey)
	if endpoint == "" || model == "" || apiKey == "" {
		return nil, errors.New("fraudguard: AI review requires an endpoint, model, and host virtual key")
	}
	if len(model) > 256 || hasControlCharacters(model) {
		return nil, errors.New("fraudguard: AI review model name is invalid")
	}
	if len(apiKey) > 4096 || hasControlCharacters(apiKey) {
		return nil, errors.New("fraudguard: AI review host virtual key is invalid")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("fraudguard: AI review endpoint must be an absolute URL without credentials, query, or fragment")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("fraudguard: AI review endpoint must use HTTP or HTTPS")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return nil, errors.New("fraudguard: cleartext AI review is allowed only on loopback; use HTTPS for remote Bifrost hosts")
	}
	if !strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1/chat/completions") {
		return nil, errors.New("fraudguard: AI review endpoint must end in /v1/chat/completions")
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if timeout > 30*time.Second {
		return nil, errors.New("fraudguard: AI review timeout cannot exceed 30 seconds")
	}

	return &BifrostHTTPAIAnalyzer{
		endpoint: parsed.String(),
		model:    model,
		apiKey:   apiKey,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// NewBifrostHTTPAIAnalyzerFromEnvironment builds the optional dynamic-plugin
// adapter from deployment environment variables and returns the internal ID
// of its dedicated host virtual key. The caller must exempt that ID from
// FraudGuard so the review request cannot recursively trigger another review.
func NewBifrostHTTPAIAnalyzerFromEnvironment(timeout time.Duration) (AIAnalyzer, string, error) {
	endpoint := strings.TrimSpace(os.Getenv(envAIReviewEndpoint))
	model := strings.TrimSpace(os.Getenv(envAIReviewModel))
	apiKey := os.Getenv(envAIReviewAPIKey)
	keyID := strings.TrimSpace(os.Getenv(envAIReviewKeyID))
	if endpoint == "" || model == "" || strings.TrimSpace(apiKey) == "" || keyID == "" {
		return nil, "", errors.New("fraudguard: AI review needs FRAUDGUARD_AI_REVIEW_ENDPOINT, FRAUDGUARD_AI_REVIEW_MODEL, FRAUDGUARD_AI_REVIEW_API_KEY, and FRAUDGUARD_AI_REVIEW_VIRTUAL_KEY_ID")
	}
	if len(keyID) > 256 || hasControlCharacters(keyID) {
		return nil, "", errors.New("fraudguard: AI review virtual-key ID is invalid")
	}
	analyzer, err := NewBifrostHTTPAIAnalyzer(BifrostHTTPAIAnalyzerConfig{
		Endpoint: endpoint,
		Model:    model,
		APIKey:   apiKey,
		Timeout:  timeout,
	})
	if err != nil {
		return nil, "", err
	}
	return analyzer, keyID, nil
}

func (a *BifrostHTTPAIAnalyzer) Analyze(ctx context.Context, input AIAnalysisInput) (AIAnalysisResult, error) {
	if a == nil || a.client == nil {
		return AIAnalysisResult{}, errors.New("fraudguard: AI review adapter is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if input.Score < 0 || input.Score > 100 || input.Threshold < 0 || input.Threshold > 100 ||
		len(input.Signals) == 0 || len(input.Signals) > maxAIReviewSignals {
		return AIAnalysisResult{}, errors.New("fraudguard: AI review finding summary is invalid")
	}

	type safeSignal struct {
		Name   string `json:"name"`
		Weight int    `json:"weight"`
	}
	signals := make([]safeSignal, 0, len(input.Signals))
	for _, signal := range input.Signals {
		if _, allowed := allowedAIReviewSignals[signal.Name]; !allowed ||
			signal.Weight < 0 || signal.Weight > 100 {
			return AIAnalysisResult{}, errors.New("fraudguard: AI review signal is invalid")
		}
		// Send only stable signal names and weights. Signal explanations are
		// intentionally omitted from the model request.
		signals = append(signals, safeSignal{Name: signal.Name, Weight: signal.Weight})
	}
	summary, err := json.Marshal(struct {
		Score     int          `json:"score"`
		Threshold int          `json:"threshold"`
		Alert     bool         `json:"alert"`
		Warmup    bool         `json:"warmup"`
		Signals   []safeSignal `json:"signals"`
	}{
		Score: input.Score, Threshold: input.Threshold, Alert: input.Alert,
		Warmup: input.Warmup, Signals: signals,
	})
	if err != nil {
		return AIAnalysisResult{}, errors.New("fraudguard: could not encode AI review summary")
	}
	body, err := json.Marshal(struct {
		Model       string `json:"model"`
		Temperature int    `json:"temperature"`
		MaxTokens   int    `json:"max_tokens"`
		Messages    []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model: a.model, Temperature: 0, MaxTokens: 160,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "system", Content: aiReviewSystemMessage},
			{Role: "user", Content: string(summary)},
		},
	})
	if err != nil || len(body) > maxAIReviewRequestBytes {
		return AIAnalysisResult{}, errors.New("fraudguard: AI review request exceeds its size limit")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return AIAnalysisResult{}, errors.New("fraudguard: could not prepare AI review request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return AIAnalysisResult{}, errors.New("fraudguard: Bifrost AI review request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return AIAnalysisResult{}, fmt.Errorf("fraudguard: Bifrost AI review returned HTTP %d", resp.StatusCode)
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxAIReviewResponseBytes+1))
	if err != nil || len(responseBody) > maxAIReviewResponseBytes {
		return AIAnalysisResult{}, errors.New("fraudguard: Bifrost AI review response is unavailable or too large")
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &completion); err != nil || len(completion.Choices) == 0 {
		return AIAnalysisResult{}, errors.New("fraudguard: Bifrost AI review response is invalid")
	}
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	var result AIAnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return AIAnalysisResult{}, errors.New("fraudguard: Bifrost AI review did not return the required JSON advisory")
	}
	result, ok := normalizeAIAnalysisResult(result)
	if !ok {
		return AIAnalysisResult{}, errors.New("fraudguard: Bifrost AI review advisory failed validation")
	}
	return result, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func hasControlCharacters(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
