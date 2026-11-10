// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

// ProductServices connects the request hooks to billing-safe usage, plan policy,
// audit, and webhook services. All external work runs outside the request hook
// on a bounded queue.
type ProductServices struct {
	TenantResolver func(*schemas.BifrostContext) string
	UsageLedger    UsageLedger
	PlanResolver   TenantTierResolver
	Policies       *TierPolicyCatalog
	AuditLog       AuditLog
	Webhooks       AuditWebhookSender
	QueueSize      int
	Workers        int
	Timeout        time.Duration
	PolicyCacheTTL time.Duration
	MaxCachedPolicies int
}

type ProductMetrics struct {
	EventsQueued       uint64 `json:"events_queued"`
	EventsDropped      uint64 `json:"events_dropped"`
	EventsFailed       uint64 `json:"events_failed"`
	MeteringFailed     uint64 `json:"metering_failed"`
	PolicyLookupFailed uint64 `json:"policy_lookup_failed"`
	PolicyCacheMisses  uint64 `json:"policy_cache_misses"`
	MissingTenant      uint64 `json:"missing_tenant"`
	AuditFailed        uint64 `json:"audit_failed"`
	WebhookSent        uint64 `json:"webhook_sent"`
	WebhookFailed      uint64 `json:"webhook_failed"`
}

type ProductRuntime struct {
	services ProductServices
	queue    chan productEvent
	mu       sync.RWMutex
	closed   bool
	wg       sync.WaitGroup
	policyCache *tierPolicyLRU

	queued        atomic.Uint64
	dropped       atomic.Uint64
	failed        atomic.Uint64
	meteringFail  atomic.Uint64
	policyFail    atomic.Uint64
	policyMiss    atomic.Uint64
	missingTenant atomic.Uint64
	auditFail     atomic.Uint64
	webhookSent   atomic.Uint64
	webhookFail   atomic.Uint64
}

type productEvent struct {
	tenantID string
	usage    *UsageEvent
	finding  *Finding
	eventID  string
}

type cachedPolicy struct {
	policy    TierPolicy
	expiresAt time.Time
}

func NewProductRuntime(services ProductServices) (*ProductRuntime, error) {
	if services.TenantResolver == nil {
		return nil, errors.New("fraudguard: trusted tenant resolver is required for product services")
	}
	if services.UsageLedger == nil && services.AuditLog == nil {
		return nil, errors.New("fraudguard: product services require a usage ledger or audit log")
	}
	if services.Webhooks != nil && services.AuditLog == nil {
		return nil, errors.New("fraudguard: webhook alerts require an audit log")
	}
	if (services.PlanResolver == nil) != (services.Policies == nil) {
		return nil, errors.New("fraudguard: plan resolver and policy catalog must be configured together")
	}
	if services.PlanResolver != nil && services.UsageLedger == nil {
		return nil, errors.New("fraudguard: tier usage evaluation requires a usage ledger")
	}
	if services.QueueSize <= 0 {
		services.QueueSize = 512
	}
	if services.QueueSize > 8192 {
		return nil, errors.New("fraudguard: product event queue cannot exceed 8192")
	}
	if services.Workers <= 0 {
		services.Workers = 2
	}
	if services.Workers > 32 {
		return nil, errors.New("fraudguard: product worker count cannot exceed 32")
	}
	if services.Timeout <= 0 {
		services.Timeout = 2 * time.Second
	}
	if services.PolicyCacheTTL <= 0 {
		services.PolicyCacheTTL = 30 * time.Second
	}
	if services.PolicyCacheTTL > 10*time.Minute {
		return nil, errors.New("fraudguard: policy cache TTL cannot exceed 10 minutes")
	}
	if services.MaxCachedPolicies <= 0 {
		services.MaxCachedPolicies = defaultMaxCachedPolicies
	}
	if services.MaxCachedPolicies > maxCachedPolicies {
		return nil, errors.New("fraudguard: policy cache capacity cannot exceed 100000")
	}
	policyCache, err := newTierPolicyLRU(services.MaxCachedPolicies)
	if err != nil {
		return nil, err
	}
	runtime := &ProductRuntime{
		services: services,
		queue:    make(chan productEvent, services.QueueSize),
		policyCache: policyCache,
	}
	for range services.Workers {
		runtime.wg.Add(1)
		go runtime.worker()
	}
	return runtime, nil
}

// CachedPolicy returns a recently resolved plan without performing network I/O.
// Request hooks use only this method so Redis availability cannot add latency
// or change request flow.
func (r *ProductRuntime) CachedPolicy(tenantID string) (TierPolicy, bool) {
	if r == nil || r.services.PlanResolver == nil || strings.TrimSpace(tenantID) == "" {
		return TierPolicy{}, false
	}
	key := policyCacheKey(tenantID)
	entry, ok := r.policyCache.get(key, time.Now())
	if !ok {
		return TierPolicy{}, false
	}
	return entry.policy, true
}

func (r *ProductRuntime) NotePolicyCacheMiss() {
	if r != nil {
		r.policyMiss.Add(1)
	}
}

func (r *ProductRuntime) NoteMissingTenant() {
	if r != nil {
		r.missingTenant.Add(1)
	}
}

func (r *ProductRuntime) NotePolicyLookupFailure() {
	if r != nil {
		r.policyFail.Add(1)
	}
}

// ResolvePolicy refreshes a plan using the trusted resolver. It is intended for
// background product processing or non-inference host integrations, not hooks.
func (r *ProductRuntime) ResolvePolicy(ctx context.Context, tenantID string) (TierPolicy, bool, error) {
	if r == nil || r.services.PlanResolver == nil {
		return TierPolicy{}, false, nil
	}
	if cached, ok := r.CachedPolicy(tenantID); ok {
		return cached, true, nil
	}
	tier, err := r.services.PlanResolver.ResolveTenantTier(ctx, tenantID)
	if err != nil {
		return TierPolicy{}, false, err
	}
	policy, err := r.services.Policies.Resolve(tier)
	if err != nil {
		return TierPolicy{}, false, err
	}
	r.policyCache.set(policyCacheKey(tenantID), cachedPolicy{
		policy: policy, expiresAt: time.Now().Add(r.services.PolicyCacheTTL),
	})
	return policy, true, nil
}

func (r *ProductRuntime) SubmitUsage(event UsageEvent) bool {
	if r == nil {
		return false
	}
	if strings.TrimSpace(event.TenantID) == "" {
		return false
	}
	copy := event
	return r.submit(productEvent{tenantID: event.TenantID, usage: &copy, eventID: event.EventID})
}

func (r *ProductRuntime) SubmitFinding(tenantID, eventID string, finding Finding) bool {
	if r == nil || strings.TrimSpace(tenantID) == "" || len(finding.Signals) == 0 {
		return false
	}
	copy := finding
	copy.Signals = cloneSignals(finding.Signals)
	return r.submit(productEvent{tenantID: tenantID, finding: &copy, eventID: eventID})
}

func (r *ProductRuntime) submit(event productEvent) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		r.dropped.Add(1)
		return false
	}
	select {
	case r.queue <- event:
		r.queued.Add(1)
		return true
	default:
		r.dropped.Add(1)
		return false
	}
}

func (r *ProductRuntime) worker() {
	defer r.wg.Done()
	for event := range r.queue {
		if err := r.process(event); err != nil {
			r.failed.Add(1)
		}
	}
}

func (r *ProductRuntime) process(event productEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.services.Timeout)
	defer cancel()

	var totals UsageTotals
	if event.usage != nil && r.services.UsageLedger != nil {
		if err := r.services.UsageLedger.RecordUsage(ctx, *event.usage); err != nil {
			r.meteringFail.Add(1)
			return fmt.Errorf("record usage: %w", err)
		}
		var err error
		totals, err = r.services.UsageLedger.UsageForMonth(ctx, event.tenantID, event.usage.OccurredAt.UTC().Format("2006-01"))
		if err != nil {
			r.meteringFail.Add(1)
			return fmt.Errorf("read usage totals: %w", err)
		}
	}

	var policy *TierPolicy
	if r.services.PlanResolver != nil {
		resolved, found, err := r.ResolvePolicy(ctx, event.tenantID)
		if err != nil {
			r.policyFail.Add(1)
			return fmt.Errorf("resolve plan: %w", err)
		}
		if found {
			policy = &resolved
		}
	}

	if event.finding != nil {
		finding := *event.finding
		webhookAllowed := true
		if policy != nil {
			finding.Alert = policy.AlertForScore(finding.Score)
			finding.Threshold = policy.AlertThreshold
			webhookAllowed = policy.Features.WebhookAlerts
		}
		record := AuditEvent{
			ID:         event.eventID,
			OccurredAt: time.Now().UTC(),
			Kind:       "risk_finding",
			Score:      finding.Score,
			Threshold:  finding.Threshold,
			Alert:      finding.Alert,
			Signals:    cloneSignals(finding.Signals),
		}
		if policy != nil {
			record.PlanTier = policy.Tier
		}
		if err := r.appendAuditAndNotify(ctx, event.tenantID, record, finding.Alert && webhookAllowed); err != nil {
			return err
		}
	}

	if event.usage != nil && policy != nil {
		evaluation := policy.EvaluateUsage(totals)
		if evaluation.RequestWarning || evaluation.TokenWarning || evaluation.Exceeded {
			kind := "usage_limit_warning"
			if evaluation.Exceeded {
				kind = "usage_limit_exceeded"
			}
			record := AuditEvent{
				ID:            event.eventID + ":plan",
				OccurredAt:    event.usage.OccurredAt.UTC(),
				Kind:          kind,
				Alert:         evaluation.Exceeded,
				PlanTier:      policy.Tier,
				PolicyOutcome: evaluation.Outcome,
				Requests:      totals.Requests,
				TotalTokens:   totals.PromptTokens + totals.CompletionTokens,
			}
			webhookAllowed := policy.Features.WebhookAlerts
			if err := r.appendAuditAndNotify(ctx, event.tenantID, record, evaluation.Exceeded && webhookAllowed); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ProductRuntime) appendAuditAndNotify(ctx context.Context, tenantID string, event AuditEvent, alert bool) error {
	if r.services.AuditLog == nil {
		return nil
	}
	record, err := r.services.AuditLog.AppendAudit(ctx, tenantID, event)
	if errors.Is(err, ErrDuplicateAuditEvent) {
		return nil
	}
	if err != nil {
		r.auditFail.Add(1)
		return fmt.Errorf("append audit event: %w", err)
	}
	if !alert || r.services.Webhooks == nil {
		return nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := r.services.Webhooks.Send(ctx, tenantID, record); err == nil {
			r.webhookSent.Add(1)
			return nil
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				r.webhookFail.Add(1)
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
			}
		}
	}
	r.webhookFail.Add(1)
	return errors.New("fraudguard: webhook delivery failed after retries")
}

func (r *ProductRuntime) Metrics() ProductMetrics {
	if r == nil {
		return ProductMetrics{}
	}
	return ProductMetrics{
		EventsQueued:       r.queued.Load(),
		EventsDropped:      r.dropped.Load(),
		EventsFailed:       r.failed.Load(),
		MeteringFailed:     r.meteringFail.Load(),
		PolicyLookupFailed: r.policyFail.Load(),
		PolicyCacheMisses:  r.policyMiss.Load(),
		MissingTenant:      r.missingTenant.Load(),
		AuditFailed:        r.auditFail.Load(),
		WebhookSent:        r.webhookSent.Load(),
		WebhookFailed:      r.webhookFail.Load(),
	}
}

func (r *ProductRuntime) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	close(r.queue)
	r.mu.Unlock()
	r.wg.Wait()
}

func policyCacheKey(tenantID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(tenantID)))
	return hex.EncodeToString(sum[:])
}
