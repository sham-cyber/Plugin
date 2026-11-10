// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"go.opentelemetry.io/otel/attribute"
)

const PluginName = "fraudguard"

type contextHandleKey struct{}
type credentialStatusContextKey struct{}
type tenantIDContextKey struct{}
type productRequestContextKey struct{}

type productRequestState struct {
	tenantID      string
	eventID       string
	occurredAt    time.Time
	checkLatency  time.Duration
	preFinding    Finding
	hasPreFinding bool
}

// SetTrustedCredentialStatus attaches authoritative key-state results for the
// current request. Call it only from trusted authentication or key-management
// code, never from caller-controlled request headers or payload fields.
func SetTrustedCredentialStatus(ctx *schemas.BifrostContext, status CredentialStatus) {
	if ctx != nil {
		ctx.SetValue(credentialStatusContextKey{}, status)
	}
}

// SetTrustedTenantID attaches a tenant identifier resolved by authenticated
// host code. Never populate it from a request header or caller-controlled body.
func SetTrustedTenantID(ctx *schemas.BifrostContext, tenantID string) {
	if ctx != nil && strings.TrimSpace(tenantID) != "" {
		ctx.SetValue(tenantIDContextKey{}, strings.TrimSpace(tenantID))
	}
}

func TrustedTenantIDFromContext(ctx *schemas.BifrostContext) string {
	if ctx == nil {
		return ""
	}
	tenantID, _ := ctx.Value(tenantIDContextKey{}).(string)
	return strings.TrimSpace(tenantID)
}

// Plugin adapts the engine to Bifrost's schemas.LLMPlugin hook contract.
// Its hooks are deliberately observe-only: they never mutate or short-circuit requests.
type Plugin struct {
	engine            *Engine
	product           *ProductRuntime
	productCloser     io.Closer
	telemetry         *Telemetry
	budgetMu          sync.RWMutex
	budget            *BudgetMonitor
	configReloader    *ConfigReloader
	configWatchCancel context.CancelFunc
	configWatchDone   chan struct{}
}

// New constructs a Bifrost plugin and its bounded in-memory detector.
func New(config Config) (*Plugin, error) {
	engine, err := NewEngine(config)
	if err != nil {
		return nil, err
	}
	return &Plugin{engine: engine, telemetry: NewTelemetry()}, nil
}

// NewWithProductServices constructs the detector with optional metering,
// tier-policy, audit, and webhook services. The services run asynchronously;
// queue overflow is visible in ProductMetrics and never changes request flow.
func NewWithProductServices(config Config, services ProductServices) (*Plugin, error) {
	engine, err := NewEngine(config)
	if err != nil {
		return nil, err
	}
	product, err := NewProductRuntime(services)
	if err != nil {
		_ = engine.Close()
		return nil, err
	}
	return &Plugin{engine: engine, product: product, telemetry: NewTelemetry()}, nil
}

func (p *Plugin) GetName() string { return PluginName }

func (p *Plugin) Cleanup() error {
	if p.configWatchCancel != nil {
		p.configWatchCancel()
	}
	if p.configWatchDone != nil {
		<-p.configWatchDone
	}
	if p.product != nil {
		p.product.Close()
	}
	var closeErr error
	if p.engine != nil {
		closeErr = p.engine.Close()
	}
	if p.productCloser != nil {
		closeErr = errors.Join(closeErr, p.productCloser.Close())
	}
	if p.telemetry != nil {
		closeErr = errors.Join(closeErr, p.telemetry.Close())
	}
	return closeErr
}

// ConfigReloadStatus reports hot-reload health without exposing configuration
// values or exemption IDs.
func (p *Plugin) ConfigReloadStatus() RuntimeConfigStatus {
	if p == nil || p.configReloader == nil {
		return RuntimeConfigStatus{}
	}
	return p.configReloader.Status()
}

func (p *Plugin) startConfigWatcher(path string, interval time.Duration) error {
	reloader, err := NewConfigReloader(p.engine)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	errorsOut, err := reloader.WatchFile(ctx, path, interval)
	if err != nil {
		cancel()
		return err
	}
	p.configReloader = reloader
	p.configWatchCancel = cancel
	p.configWatchDone = make(chan struct{})
	go func() {
		defer close(p.configWatchDone)
		for range errorsOut {
			// Reload failures are visible through ConfigReloadStatus. Drain the
			// channel so repeated bad edits cannot block the watcher.
		}
	}()
	return nil
}

// PreRequestHook is required by schemas.LLMPlugin. FraudGuard does not route or mutate.
func (p *Plugin) PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

// PreLLMHook records safe metadata, optionally inspects bounded user text for
// opt-in signals, and always continues processing without retaining that text.
func (p *Plugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if ctx == nil || req == nil {
		return req, nil, nil
	}
	traceCtx, hookSpan := startFraudGuardSpan(ctx, "fraudguard.pre_llm")
	defer hookSpan.End()
	started := time.Now()
	var assessment Assessment
	profiled := false
	defer func() {
		if p.telemetry != nil {
			p.telemetry.RecordEvaluation(time.Since(started), assessment.Finding, profiled)
		}
	}()
	tenantID := ""
	var tierPolicy TierPolicy
	hasTierPolicy := false
	if p.product != nil {
		tenantID = p.product.services.TenantResolver(ctx)
		if tenantID == "" {
			p.product.NoteMissingTenant()
		} else if p.product.services.PlanResolver != nil {
			tierPolicy, hasTierPolicy = p.product.CachedPolicy(tenantID)
			if !hasTierPolicy {
				p.product.NotePolicyCacheMiss()
			}
		}
	}
	// This is the authenticated internal row ID set by Bifrost governance, never
	// the raw x-bf-vk header credential.
	keyID := resolvedInternalVirtualKeyID(ctx)
	var handle Handle
	if keyID != "" {
		profiled = true
		engineConfig := p.engine.configSnapshot()
		credentialStatus, _ := ctx.Value(credentialStatusContextKey{}).(CredentialStatus)
		if p.product != nil && p.product.services.PlanResolver != nil &&
			(!hasTierPolicy || !tierPolicy.Features.KeyStatusSignals) {
			credentialStatus = CredentialStatus{}
		}
		provider, model, _ := req.GetRequestFields()
		cohortID := ""
		allowCrossKey := engineConfig.EnableCrossKeyCorrelation
		if p.product != nil && p.product.services.PlanResolver != nil &&
			(!hasTierPolicy || !tierPolicy.Features.CrossKeySignals) {
			allowCrossKey = false
		}
		if allowCrossKey {
			cohortID = resolvedUserID(ctx)
		}
		prompt := promptInspection{}
		contextAssessment := ContextGuardAssessment{}
		allowPromptSignals := engineConfig.EnablePromptOverrideSignals ||
			engineConfig.EnablePromptLengthSignal || engineConfig.EnableContextGuard ||
			engineConfig.EnableCustomPromptRules
		if p.product != nil && p.product.services.PlanResolver != nil &&
			(!hasTierPolicy || !tierPolicy.Features.PromptSignals) {
			allowPromptSignals = false
		}
		if allowPromptSignals {
			_, scanSpan := startFraudGuardSpan(traceCtx, "fraudguard.prompt_inspection")
			if engineConfig.EnablePromptOverrideSignals || engineConfig.EnablePromptLengthSignal {
				prompt = inspectBifrostRequestWithRules(
					req,
					engineConfig.MaxPromptScanBytes,
					engineConfig.EnablePromptOverrideSignals,
					customPromptRulesForConfig(engineConfig),
				)
			} else if engineConfig.EnableCustomPromptRules {
				prompt = inspectBifrostRequestWithRules(
					req,
					engineConfig.MaxPromptScanBytes,
					false,
					customPromptRulesForConfig(engineConfig),
				)
			}
			if engineConfig.EnableContextGuard {
				contextAssessment = inspectBifrostContext(req, ContextGuardOptions{
					MaxMessages:  engineConfig.MaxContextMessages,
					MaxScanBytes: engineConfig.MaxContextScanBytes,
				})
			}
			scanSpan.SetAttributes(
				attribute.Bool("fraudguard.prompt.override_check_enabled", engineConfig.EnablePromptOverrideSignals),
				attribute.Bool("fraudguard.prompt.context_guard_enabled", engineConfig.EnableContextGuard),
				attribute.Int64("fraudguard.prompt.user_text_bytes", prompt.userBytes),
				attribute.Bool("fraudguard.prompt.override_signal", prompt.overrideDetected),
				attribute.Bool("fraudguard.prompt.custom_rule_signal", prompt.customRuleMatched),
				attribute.Bool("fraudguard.prompt.context_signal",
					contextAssessment.ProgressiveJailbreak ||
						contextAssessment.DataExfiltration ||
						contextAssessment.SchemaManipulation),
			)
			scanSpan.End()
		}
		startRequest := Request{
			VirtualKeyID:                keyID,
			CohortID:                    cohortID,
			Provider:                    string(provider),
			Model:                       model,
			PromptOverrideDetected:      prompt.overrideDetected,
			CustomPromptRuleMatched:     prompt.customRuleMatched,
			PromptBytes:                 prompt.userBytes,
			ContextProgressiveJailbreak: contextAssessment.ProgressiveJailbreak,
			ContextDataExfiltration:     contextAssessment.DataExfiltration,
			ContextSchemaManipulation:   contextAssessment.SchemaManipulation,
			CredentialStatus:            credentialStatus,
		}
		if p.product != nil && p.product.services.PlanResolver != nil && hasTierPolicy {
			threshold := tierPolicy.AlertThreshold
			startRequest.AlertThresholdOverride = &threshold
		}
		if p.product != nil && p.product.services.PlanResolver != nil {
			allowAIReview := hasTierPolicy && tierPolicy.Features.AIReview
			startRequest.AIReviewOverride = &allowAIReview
		}
		_, scoringSpan := startFraudGuardSpan(traceCtx, "fraudguard.risk_scoring")
		handle, assessment = p.engine.Start(startRequest)
		scoringSpan.SetAttributes(
			attribute.Int("fraudguard.risk.score", assessment.Finding.Score),
			attribute.Bool("fraudguard.risk.alert", assessment.Finding.Alert),
			attribute.Int("fraudguard.risk.signal_count", len(assessment.Finding.Signals)),
		)
		scoringSpan.End()
		if handle.profileID != "" {
			ctx.SetValue(contextHandleKey{}, handle)
		}
	}
	if p.product != nil && tenantID != "" {
		eventID := newProductEventID()
		if eventID != "" {
			state := productRequestState{
				tenantID:      tenantID,
				eventID:       eventID,
				occurredAt:    started.UTC(),
				checkLatency:  time.Since(started),
				preFinding:    assessment.Finding,
				hasPreFinding: len(assessment.Finding.Signals) > 0,
			}
			ctx.SetValue(productRequestContextKey{}, state)
		}
	}
	hookSpan.SetAttributes(attribute.Bool("fraudguard.profiled", profiled))
	return req, nil, nil
}

// resolvedUserID uses only the settled grant identity. It deliberately does not
// inspect request headers or fall back to a virtual-key owner that Bifrost does
// not expose as an authenticated user identity.
func resolvedUserID(ctx *schemas.BifrostContext) string {
	if ctx == nil {
		return ""
	}
	grant := ctx.Grant()
	if grant == nil {
		return ""
	}
	identity := grant.Identity()
	if identity == nil {
		return ""
	}
	user := identity.User()
	if user == nil {
		return ""
	}
	return user.ID
}

// PostLLMHook records normalized token totals and a boolean outcome. It never reads
// prompt/response content or the error message, and never alters the Bifrost result.
func (p *Plugin) PostLLMHook(ctx *schemas.BifrostContext, response *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	if ctx == nil {
		return response, bifrostErr, nil
	}
	_, hookSpan := startFraudGuardSpan(ctx, "fraudguard.post_llm")
	defer hookSpan.End()
	hookSpan.SetAttributes(attribute.Bool("fraudguard.upstream_error", bifrostErr != nil))
	if p.telemetry != nil {
		defer p.telemetry.RecordCompletion()
	}
	var assessment Assessment
	if handle, ok := ctx.Value(contextHandleKey{}).(Handle); ok && handle.profileID != "" {
		completion := Completion{Failed: bifrostErr != nil}
		if response != nil {
			if usage := response.NormalizedUsage(); usage != nil {
				completion.InputTokens = int64(usage.PromptTokens)
				completion.OutputTokens = int64(usage.CompletionTokens)
			}
		}
		assessment = p.engine.Complete(handle, completion)
		if p.telemetry != nil {
			p.telemetry.RecordFinding(assessment.Finding)
		}
	}
	if p.product != nil {
		state, ok := ctx.Value(productRequestContextKey{}).(productRequestState)
		if ok && state.tenantID != "" {
			var promptTokens, completionTokens int64
			if response != nil {
				if usage := response.NormalizedUsage(); usage != nil {
					promptTokens = int64(usage.PromptTokens)
					completionTokens = int64(usage.CompletionTokens)
				}
			}
			findings := int64(0)
			alerts := int64(0)
			if state.hasPreFinding {
				findings++
				if state.preFinding.Alert {
					alerts++
				}
				p.product.SubmitFinding(state.tenantID, state.eventID+":pre", state.preFinding)
			}
			if len(assessment.Finding.Signals) > 0 {
				findings++
				if assessment.Finding.Alert {
					alerts++
				}
				p.product.SubmitFinding(state.tenantID, state.eventID+":post", assessment.Finding)
			}
			p.product.SubmitUsage(UsageEvent{
				EventID:           state.eventID,
				TenantID:          state.tenantID,
				OccurredAt:        state.occurredAt,
				PromptTokens:      promptTokens,
				CompletionTokens:  completionTokens,
				FraudCheckLatency: state.checkLatency,
				FindingCount:      findings,
				AlertCount:        alerts,
			})
		}
	}
	return response, bifrostErr, nil
}

// Engine exposes aggregate metrics and deterministic standalone scoring to operators.
func (p *Plugin) Engine() *Engine { return p.engine }

func (p *Plugin) ProductMetrics() ProductMetrics {
	if p == nil || p.product == nil {
		return ProductMetrics{}
	}
	return p.product.Metrics()
}

// RedisCircuitStatus reports only aggregate connectivity state, never Redis
// addresses, tenant IDs, or stored usage/audit values.
func (p *Plugin) RedisCircuitStatus() CircuitBreakerStatus {
	if p == nil || p.product == nil {
		return CircuitBreakerStatus{State: CircuitClosed}
	}
	if store, ok := p.product.services.UsageLedger.(interface {
		CircuitStatus() CircuitBreakerStatus
	}); ok {
		return store.CircuitStatus()
	}
	return CircuitBreakerStatus{State: CircuitClosed}
}

func newProductEventID() string {
	var raw [16]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(raw[:])
}

// RecordPayment accepts one trusted transaction event from a host payment
// integration. It only emits an observe-only finding; it never authorizes,
// declines, or changes a payment.
func (p *Plugin) RecordPayment(event PaymentEvent) Assessment {
	if p.product != nil && p.product.services.PlanResolver != nil {
		if strings.TrimSpace(event.TenantID) == "" {
			p.product.NoteMissingTenant()
			return Assessment{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), p.product.services.Timeout)
		policy, found, err := p.product.ResolvePolicy(ctx, event.TenantID)
		cancel()
		if err != nil || !found {
			p.product.NotePolicyLookupFailure()
			return Assessment{}
		}
		if !policy.Features.PaymentSignals {
			return Assessment{}
		}
		if policy.MaxPaymentsPerWindow > 0 {
			event.MaxPaymentsPerWindowOverride = policy.MaxPaymentsPerWindow
		}
		threshold := policy.AlertThreshold
		event.AlertThresholdOverride = &threshold
		allowAIReview := policy.Features.AIReview
		event.AIReviewOverride = &allowAIReview
	}
	assessment := p.engine.RecordPayment(event)
	if p.product != nil && event.TenantID != "" && len(assessment.Finding.Signals) > 0 {
		eventID := newProductEventID()
		if eventID != "" {
			p.product.SubmitFinding(event.TenantID, eventID+":payment", assessment.Finding)
		}
	}
	return assessment
}

var _ schemas.LLMPlugin = (*Plugin)(nil)
