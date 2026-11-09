// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"strings"
	"unicode"
)

const maxAIExplanationRunes = 280

// AIConclusion is an advisory label returned by a host-supplied model adapter.
// It never changes the deterministic score or request handling.
type AIConclusion string

const (
	AIConclusionReviewSuggested  AIConclusion = "review_suggested"
	AIConclusionNoAddedConcern   AIConclusion = "no_added_concern"
	AIConclusionInsufficientData AIConclusion = "insufficient_data"
)

// AIAnalysisInput contains only the deterministic finding summary. It has no
// prompt, payload, virtual-key, payment-subject, provider, or model identifiers.
type AIAnalysisInput struct {
	Score     int      `json:"score"`
	Threshold int      `json:"threshold"`
	Alert     bool     `json:"alert"`
	Warmup    bool     `json:"warmup"`
	Signals   []Signal `json:"signals"`
}

// AIAnalysisResult is a short advisory from the host model. Confidence is
// descriptive only and must not be treated as a calibrated fraud probability.
type AIAnalysisResult struct {
	Conclusion  AIConclusion `json:"conclusion"`
	Confidence  int          `json:"confidence"`
	Explanation string       `json:"explanation"`
}

// AIReviewEvent pairs the advisory with its original deterministic finding.
// It contains no request or customer identifiers.
type AIReviewEvent struct {
	Finding  Finding          `json:"finding"`
	Analysis AIAnalysisResult `json:"analysis"`
}

// AIAnalyzer is implemented by the Bifrost host. Its adapter should call an
// isolated host-configured model client that does not include FraudGuard, to
// prevent recursive analysis. It must honor the provided timeout context.
type AIAnalyzer interface {
	Analyze(context.Context, AIAnalysisInput) (AIAnalysisResult, error)
}

// AIAnalyzerFunc adapts a function to the AIAnalyzer interface.
type AIAnalyzerFunc func(context.Context, AIAnalysisInput) (AIAnalysisResult, error)

func (f AIAnalyzerFunc) Analyze(ctx context.Context, input AIAnalysisInput) (AIAnalysisResult, error) {
	return f(ctx, input)
}

type aiAnalysisJob struct {
	finding Finding
	input   AIAnalysisInput
}

func (e *Engine) enqueueAIReview(finding Finding) {
	if !e.cfg.EnableAIAnalysis || len(finding.Signals) == 0 || finding.Score < e.cfg.AIAnalysisMinScore {
		return
	}
	job := aiAnalysisJob{
		finding: cloneFinding(finding),
		input: AIAnalysisInput{
			Score:     finding.Score,
			Threshold: finding.Threshold,
			Alert:     finding.Alert,
			Warmup:    finding.Warmup,
			Signals:   append([]Signal(nil), finding.Signals...),
		},
	}

	e.aiMu.Lock()
	if e.aiClosed {
		e.aiMu.Unlock()
		e.recordAIReviewMetric(func(m *Metrics) { m.AIReviewsDropped++ })
		return
	}
	select {
	case e.aiQueue <- job:
		e.aiMu.Unlock()
		e.recordAIReviewMetric(func(m *Metrics) { m.AIReviewsQueued++ })
	default:
		e.aiMu.Unlock()
		e.recordAIReviewMetric(func(m *Metrics) { m.AIReviewsDropped++ })
	}
}

func (e *Engine) runAIWorker() {
	defer e.aiWG.Done()
	for job := range e.aiQueue {
		ctx, cancel := context.WithTimeout(context.Background(), e.cfg.AIAnalysisTimeout)
		result, err := e.cfg.AIAnalyzer.Analyze(ctx, job.input)
		cancel()
		if err != nil {
			e.recordAIReviewMetric(func(m *Metrics) { m.AIReviewsFailed++ })
			continue
		}
		result, ok := normalizeAIAnalysisResult(result)
		if !ok {
			e.recordAIReviewMetric(func(m *Metrics) { m.AIReviewsFailed++ })
			continue
		}
		e.recordAIReviewMetric(func(m *Metrics) { m.AIReviewsCompleted++ })
		e.cfg.OnAIReview(AIReviewEvent{
			Finding:  job.finding,
			Analysis: result,
		})
	}
}

func (e *Engine) recordAIReviewMetric(update func(*Metrics)) {
	e.mu.Lock()
	update(&e.metrics)
	e.mu.Unlock()
}

// Close stops accepting AI review work, drains the bounded queue, and waits for
// adapters to finish. Host adapters must honor the timeout context.
func (e *Engine) Close() error {
	if e.aiQueue == nil {
		return nil
	}
	e.aiMu.Lock()
	if !e.aiClosed {
		e.aiClosed = true
		close(e.aiQueue)
	}
	e.aiMu.Unlock()
	e.aiWG.Wait()
	return nil
}

func cloneFinding(finding Finding) Finding {
	finding.Signals = append([]Signal(nil), finding.Signals...)
	return finding
}

func normalizeAIAnalysisResult(result AIAnalysisResult) (AIAnalysisResult, bool) {
	switch result.Conclusion {
	case AIConclusionReviewSuggested, AIConclusionNoAddedConcern, AIConclusionInsufficientData:
	default:
		return AIAnalysisResult{}, false
	}
	if result.Confidence < 0 || result.Confidence > 100 {
		return AIAnalysisResult{}, false
	}
	result.Explanation = cleanAIExplanation(result.Explanation)
	if result.Explanation == "" {
		return AIAnalysisResult{}, false
	}
	return result, true
}

func cleanAIExplanation(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	runes := []rune(strings.Join(strings.Fields(value), " "))
	if len(runes) > maxAIExplanationRunes {
		runes = runes[:maxAIExplanationRunes]
	}
	return string(runes)
}
