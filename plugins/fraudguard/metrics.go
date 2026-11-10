// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/metric"
)

var evaluationDurationBounds = [...]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}
var riskScoreBounds = [...]float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}

type HistogramSnapshot struct {
	Bounds []float64 `json:"bounds"`
	Counts []uint64  `json:"bucket_counts"`
	Count  uint64    `json:"count"`
	Sum    float64   `json:"sum"`
}

// TelemetrySnapshot contains only process-wide aggregates. It has no key,
// tenant, model, provider, prompt, request, or payment labels.
type TelemetrySnapshot struct {
	Evaluations        uint64            `json:"evaluations"`
	ProfiledRequests   uint64            `json:"profiled_requests"`
	CompletedRequests  uint64            `json:"completed_requests"`
	Findings           uint64            `json:"findings"`
	Alerts             uint64            `json:"alerts"`
	EvaluationDuration HistogramSnapshot `json:"evaluation_duration_seconds"`
	RiskScore          HistogramSnapshot `json:"risk_score"`
}

type Telemetry struct {
	mu    sync.Mutex
	state TelemetrySnapshot
	otel  *otelMetricSet
}

type otelMetricSet struct {
	evaluations  metric.Int64Counter
	profiled     metric.Int64Counter
	completed    metric.Int64Counter
	findings     metric.Int64Counter
	alerts       metric.Int64Counter
	duration     metric.Float64Histogram
	score        metric.Int64Histogram
	profiles     metric.Int64ObservableGauge
	cohorts      metric.Int64ObservableGauge
	payments     metric.Int64ObservableGauge
	registration metric.Registration
}

func NewTelemetry() *Telemetry {
	return &Telemetry{state: TelemetrySnapshot{
		EvaluationDuration: HistogramSnapshot{
			Bounds: append([]float64(nil), evaluationDurationBounds[:]...),
			Counts: make([]uint64, len(evaluationDurationBounds)+1),
		},
		RiskScore: HistogramSnapshot{
			Bounds: append([]float64(nil), riskScoreBounds[:]...),
			Counts: make([]uint64, len(riskScoreBounds)+1),
		},
	}}
}

// EnableOpenTelemetry binds process-level instruments to the host's meter.
// Call before serving traffic. It deliberately attaches no tenant or request
// attributes, which keeps cardinality bounded and identifiers private.
func (t *Telemetry) EnableOpenTelemetry(meter metric.Meter, engineMetrics func() Metrics) error {
	if t == nil || meter == nil || engineMetrics == nil {
		return errors.New("fraudguard: telemetry, OpenTelemetry meter, and engine metrics source are required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.otel != nil {
		return errors.New("fraudguard: OpenTelemetry metrics are already enabled")
	}

	instruments := &otelMetricSet{}
	var err error
	if instruments.evaluations, err = meter.Int64Counter("fraudguard.evaluations",
		metric.WithDescription("LLM plugin pre-hook evaluations"), metric.WithUnit("{request}")); err != nil {
		return err
	}
	if instruments.profiled, err = meter.Int64Counter("fraudguard.profiled_requests",
		metric.WithDescription("Evaluations with a resolved internal virtual key"), metric.WithUnit("{request}")); err != nil {
		return err
	}
	if instruments.completed, err = meter.Int64Counter("fraudguard.completed_requests",
		metric.WithDescription("Requests observed by the post-hook"), metric.WithUnit("{request}")); err != nil {
		return err
	}
	if instruments.findings, err = meter.Int64Counter("fraudguard.findings",
		metric.WithDescription("Observe-only evaluations with one or more risk signals"), metric.WithUnit("{finding}")); err != nil {
		return err
	}
	if instruments.alerts, err = meter.Int64Counter("fraudguard.alerts",
		metric.WithDescription("Observe-only findings at or above the configured threshold"), metric.WithUnit("{alert}")); err != nil {
		return err
	}
	if instruments.duration, err = meter.Float64Histogram("fraudguard.evaluation.duration",
		metric.WithDescription("FraudGuard pre-hook evaluation duration"), metric.WithUnit("s")); err != nil {
		return err
	}
	if instruments.score, err = meter.Int64Histogram("fraudguard.risk.score",
		metric.WithDescription("Risk scores observed per profiled request"), metric.WithUnit("{score}")); err != nil {
		return err
	}
	if instruments.profiles, err = meter.Int64ObservableGauge("fraudguard.active_profiles",
		metric.WithDescription("Currently retained behavioral profiles"), metric.WithUnit("{profile}")); err != nil {
		return err
	}
	if instruments.cohorts, err = meter.Int64ObservableGauge("fraudguard.active_cohorts",
		metric.WithDescription("Currently retained privacy-aware correlation cohorts"), metric.WithUnit("{cohort}")); err != nil {
		return err
	}
	if instruments.payments, err = meter.Int64ObservableGauge("fraudguard.active_payment_subjects",
		metric.WithDescription("Currently retained HMAC-keyed payment subjects"), metric.WithUnit("{subject}")); err != nil {
		return err
	}
	instruments.registration, err = meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		snapshot := engineMetrics()
		observer.ObserveInt64(instruments.profiles, int64(snapshot.Profiles))
		observer.ObserveInt64(instruments.cohorts, int64(snapshot.Cohorts))
		observer.ObserveInt64(instruments.payments, int64(snapshot.PaymentSubjects))
		return nil
	}, instruments.profiles, instruments.cohorts, instruments.payments)
	if err != nil {
		return err
	}
	t.otel = instruments
	return nil
}

func (t *Telemetry) RecordEvaluation(duration time.Duration, finding Finding, profiled bool) {
	if t == nil {
		return
	}
	if duration < 0 {
		duration = 0
	}
	seconds := duration.Seconds()
	t.mu.Lock()
	t.state.Evaluations++
	recordHistogram(&t.state.EvaluationDuration, seconds)
	instruments := t.otel
	if profiled {
		t.state.ProfiledRequests++
		recordHistogram(&t.state.RiskScore, float64(finding.Score))
	}
	if profiled && len(finding.Signals) > 0 {
		t.state.Findings++
		if finding.Alert {
			t.state.Alerts++
		}
	}
	t.mu.Unlock()

	if instruments == nil {
		return
	}
	ctx := context.Background()
	instruments.evaluations.Add(ctx, 1)
	instruments.duration.Record(ctx, seconds)
	if profiled {
		instruments.profiled.Add(ctx, 1)
		instruments.score.Record(ctx, int64(finding.Score))
	}
	if profiled && len(finding.Signals) > 0 {
		instruments.findings.Add(ctx, 1)
		if finding.Alert {
			instruments.alerts.Add(ctx, 1)
		}
	}
}

func (t *Telemetry) RecordCompletion() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.state.CompletedRequests++
	instruments := t.otel
	t.mu.Unlock()
	if instruments != nil {
		instruments.completed.Add(context.Background(), 1)
	}
}

func (t *Telemetry) RecordFinding(finding Finding) {
	if t == nil || len(finding.Signals) == 0 {
		return
	}
	t.mu.Lock()
	t.state.Findings++
	if finding.Alert {
		t.state.Alerts++
	}
	instruments := t.otel
	t.mu.Unlock()
	if instruments == nil {
		return
	}
	instruments.findings.Add(context.Background(), 1)
	if finding.Alert {
		instruments.alerts.Add(context.Background(), 1)
	}
}

func (t *Telemetry) Snapshot() TelemetrySnapshot {
	if t == nil {
		return TelemetrySnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.state
	out.EvaluationDuration.Bounds = append([]float64(nil), t.state.EvaluationDuration.Bounds...)
	out.EvaluationDuration.Counts = append([]uint64(nil), t.state.EvaluationDuration.Counts...)
	out.RiskScore.Bounds = append([]float64(nil), t.state.RiskScore.Bounds...)
	out.RiskScore.Counts = append([]uint64(nil), t.state.RiskScore.Counts...)
	return out
}

func (t *Telemetry) Close() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	instruments := t.otel
	t.otel = nil
	t.mu.Unlock()
	if instruments != nil && instruments.registration != nil {
		return instruments.registration.Unregister()
	}
	return nil
}

func recordHistogram(histogram *HistogramSnapshot, value float64) {
	index := len(histogram.Bounds)
	for i, bound := range histogram.Bounds {
		if value <= bound {
			index = i
			break
		}
	}
	histogram.Counts[index]++
	histogram.Count++
	histogram.Sum += value
}

func (p *Plugin) EnableOpenTelemetry(meter metric.Meter) error {
	if p == nil || p.engine == nil {
		return errors.New("fraudguard: plugin is not initialized")
	}
	return p.telemetry.EnableOpenTelemetry(meter, p.engine.Metrics)
}

func (p *Plugin) TelemetrySnapshot() TelemetrySnapshot {
	if p == nil {
		return TelemetrySnapshot{}
	}
	return p.telemetry.Snapshot()
}

func (p *Plugin) PrometheusText() string {
	if p == nil {
		return ""
	}
	var output strings.Builder
	snapshot := p.telemetry.Snapshot()
	engine := p.engine.Metrics()
	product := p.ProductMetrics()
	budget := p.BudgetMetrics()

	writeCounter(&output, "fraudguard_evaluations_total", "LLM plugin pre-hook evaluations.", snapshot.Evaluations)
	writeCounter(&output, "fraudguard_profiled_requests_total", "Evaluations with a resolved internal virtual key.", snapshot.ProfiledRequests)
	writeCounter(&output, "fraudguard_completed_requests_total", "Requests observed by the post-hook.", snapshot.CompletedRequests)
	writeCounter(&output, "fraudguard_findings_total", "Observe-only evaluations with one or more risk signals.", snapshot.Findings)
	writeCounter(&output, "fraudguard_alerts_total", "Observe-only findings at or above the configured threshold.", snapshot.Alerts)
	writeHistogram(&output, "fraudguard_evaluation_duration_seconds", "FraudGuard pre-hook evaluation duration in seconds.", snapshot.EvaluationDuration)
	writeHistogram(&output, "fraudguard_risk_score", "Risk scores observed per profiled request.", snapshot.RiskScore)

	writeGauge(&output, "fraudguard_active_profiles", "Currently retained behavioral profiles.", engine.Profiles)
	writeGauge(&output, "fraudguard_active_cohorts", "Currently retained privacy-aware correlation cohorts.", engine.Cohorts)
	writeGauge(&output, "fraudguard_active_payment_subjects", "Currently retained HMAC-keyed payment subjects.", engine.PaymentSubjects)
	writeCounter(&output, "fraudguard_engine_exempt_requests_total", "Requests excluded by configured internal-key exemptions.", engine.ExemptRequests)
	writeCounter(&output, "fraudguard_product_events_dropped_total", "Product events dropped from bounded background queues.", product.EventsDropped)
	writeCounter(&output, "fraudguard_product_metering_failures_total", "Usage metering service failures.", product.MeteringFailed)
	writeCounter(&output, "fraudguard_product_audit_failures_total", "Audit service failures.", product.AuditFailed)
	writeCounter(&output, "fraudguard_product_webhook_failures_total", "Webhook delivery failures after retries.", product.WebhookFailed)
	writeCounter(&output, "fraudguard_product_webhooks_sent_total", "Successful webhook deliveries.", product.WebhookSent)
	writeGauge(&output, "fraudguard_budget_tracked_periods", "Tenant-period budget aggregates retained in this process.", budget.TrackedPeriods)
	writeCounter(&output, "fraudguard_budget_observations_total", "Trusted budget usage observations.", budget.Observations)
	writeCounter(&output, "fraudguard_budget_warnings_total", "Observe-only budget warning assessments.", budget.Warnings)
	writeCounter(&output, "fraudguard_budget_exceeded_total", "Observe-only budget exceeded assessments.", budget.Exceeded)
	breaker := p.RedisCircuitStatus()
	opened := 0
	if breaker.State == CircuitOpen {
		opened = 1
	}
	writeGauge(&output, "fraudguard_redis_circuit_open", "One when optional Redis product services are circuit-open.", opened)
	return output.String()
}

func writeCounter(output *strings.Builder, name, help string, value uint64) {
	writeMetricHeader(output, name, help, "counter")
	fmt.Fprintf(output, "%s %d\n", name, value)
}

func writeGauge(output *strings.Builder, name, help string, value int) {
	writeMetricHeader(output, name, help, "gauge")
	fmt.Fprintf(output, "%s %d\n", name, value)
}

func writeMetricHeader(output *strings.Builder, name, help, kind string) {
	fmt.Fprintf(output, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
}

func writeHistogram(output *strings.Builder, name, help string, histogram HistogramSnapshot) {
	writeMetricHeader(output, name, help, "histogram")
	var cumulative uint64
	for i, bound := range histogram.Bounds {
		cumulative += histogram.Counts[i]
		fmt.Fprintf(output, "%s_bucket{le=%q} %d\n", name, strconv.FormatFloat(bound, 'g', -1, 64), cumulative)
	}
	if len(histogram.Counts) > len(histogram.Bounds) {
		cumulative += histogram.Counts[len(histogram.Bounds)]
	}
	fmt.Fprintf(output, "%s_bucket{le=\"+Inf\"} %d\n", name, histogram.Count)
	sum := histogram.Sum
	if math.IsNaN(sum) || math.IsInf(sum, 0) {
		sum = 0
	}
	fmt.Fprintf(output, "%s_sum %s\n%s_count %d\n", name, strconv.FormatFloat(sum, 'g', -1, 64), name, histogram.Count)
}
