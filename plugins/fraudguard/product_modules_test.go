// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/redis/go-redis/v9"
)

func newTestRedisProductStore(t *testing.T) (*RedisProductStore, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store, err := NewRedisProductStore(RedisStoreOptions{
		Client:    client,
		HMACKey:   []byte("test-only-hmac-key-012345678901234567890123456789"),
		Namespace: "test_fraudguard",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return store, server
}

func TestRedisUsageLedgerIsIdempotentAndHidesTenantID(t *testing.T) {
	store, server := newTestRedisProductStore(t)
	ctx := context.Background()
	event := UsageEvent{
		EventID:           "request-00000001",
		TenantID:          "sensitive-company-identifier",
		Provider:          "openai",
		Model:             "gpt-5.4-mini",
		OccurredAt:        time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
		PromptTokens:      120,
		CompletionTokens:  40,
		FraudCheckLatency: 7 * time.Millisecond,
		FindingCount:      1,
		AlertCount:        1,
	}
	if err := store.RecordUsage(ctx, event); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordUsage(ctx, event); err != nil {
		t.Fatal(err)
	}
	totals, err := store.UsageForMonth(ctx, event.TenantID, "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if totals.Requests != 1 || totals.PromptTokens != 120 || totals.CompletionTokens != 40 ||
		totals.FraudChecks != 1 || totals.FraudCheckMicros != 7000 || totals.Findings != 1 || totals.Alerts != 1 {
		t.Fatalf("unexpected usage totals: %+v", totals)
	}
	if !totals.ModelBreakdownComplete || len(totals.Models) != 1 ||
		totals.Models[0].Provider != "openai" || totals.Models[0].Model != "gpt-5.4-mini" ||
		totals.Models[0].Requests != 1 || totals.Models[0].PromptTokens != 120 ||
		totals.Models[0].CompletionTokens != 40 {
		t.Fatalf("provider/model usage was not aggregated: %+v", totals)
	}
	if totals.TenantRef == "" || totals.TenantRef == event.TenantID {
		t.Fatalf("tenant was not pseudonymized: %q", totals.TenantRef)
	}
	for _, key := range server.Keys() {
		if strings.Contains(key, event.TenantID) || strings.Contains(key, event.EventID) {
			t.Fatalf("raw tenant or event ID leaked into Redis key %q", key)
		}
	}
}

func TestRedisUsageLedgerRejectsInvalidUsageAndMonth(t *testing.T) {
	store, _ := newTestRedisProductStore(t)
	ctx := context.Background()
	if err := store.RecordUsage(ctx, UsageEvent{EventID: "invalid-00001", TenantID: "tenant", OccurredAt: time.Now(), PromptTokens: -1}); err == nil {
		t.Fatal("negative token count was accepted")
	}
	if _, err := store.UsageForMonth(ctx, "tenant", "2026-13"); err == nil {
		t.Fatal("invalid month was accepted")
	}
	if _, err := store.UsageForMonth(ctx, "", "2026-11"); !errors.Is(err, ErrInvalidTenantID) {
		t.Fatalf("expected missing-tenant error, got %v", err)
	}
}

func TestRedisPlanAssignmentAndTierPolicyStayObserveOnly(t *testing.T) {
	store, _ := newTestRedisProductStore(t)
	ctx := context.Background()
	if err := store.SetTenantTier(ctx, "tenant-abc", PlanPro); err != nil {
		t.Fatal(err)
	}
	tier, err := store.ResolveTenantTier(ctx, "tenant-abc")
	if err != nil || tier != PlanPro {
		t.Fatalf("resolved tier=%q err=%v", tier, err)
	}
	catalog, err := NewTierPolicyCatalog([]TierPolicy{{
		Tier:                PlanPro,
		MonthlyRequestLimit: 10,
		MonthlyTokenLimit:   1000,
		AlertThreshold:      45,
		WarningPercent:      80,
		Features: FeatureEntitlements{
			UsageExport:   true,
			WebhookAlerts: true,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := catalog.Resolve(tier)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewProductRuntime(ProductServices{
		TenantResolver: TrustedTenantIDFromContext,
		UsageLedger:    store,
		PlanResolver:   store,
		Policies:       catalog,
		QueueSize:      2,
		Workers:        1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	resolved, found, err := runtime.ResolvePolicy(ctx, "tenant-abc")
	if err != nil || !found || resolved.AlertThreshold != policy.AlertThreshold {
		t.Fatalf("trusted tenant policy was not resolved and cached: policy=%+v found=%t err=%v", resolved, found, err)
	}
	cached, found := runtime.CachedPolicy("tenant-abc")
	if !found || cached.Tier != PlanPro {
		t.Fatalf("cached tenant policy missing: policy=%+v found=%t", cached, found)
	}
	evaluation := policy.EvaluateUsage(UsageTotals{Requests: 11, PromptTokens: 800, CompletionTokens: 300})
	if !evaluation.Exceeded || evaluation.Outcome != "limit_exceeded_observe_only" {
		t.Fatalf("expected an observable overage, got %+v", evaluation)
	}
	if !policy.AlertForScore(45) || policy.AlertForScore(44) {
		t.Fatal("plan alert threshold was not applied inclusively")
	}
}

func TestTierPolicyUsageArithmeticSaturatesInsteadOfOverflowing(t *testing.T) {
	policy := TierPolicy{
		Tier: PlanEnterprise, MonthlyTokenLimit: math.MaxInt64,
		AlertThreshold: 60, WarningPercent: 80,
	}
	result := policy.EvaluateUsage(UsageTotals{
		PromptTokens: math.MaxInt64, CompletionTokens: 1,
	})
	if result.TokenLimitExceeded || !result.TokenWarning {
		t.Fatalf("token totals overflowed policy arithmetic: %+v", result)
	}
}

func TestUsageExportIsVersionedAndContainsNoRawTenantOrPrice(t *testing.T) {
	store, _ := newTestRedisProductStore(t)
	ctx := context.Background()
	if err := store.RecordUsage(ctx, UsageEvent{
		EventID: "export-00000001", TenantID: "tenant-private-123",
		Provider: "anthropic", Model: "claude-sonnet-5-5",
		OccurredAt:   time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		PromptTokens: 10, CompletionTokens: 5, FraudCheckLatency: time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	plan := testPlanResolver(func(context.Context, string) (PlanTier, error) { return PlanFree, nil })
	catalog, err := NewTierPolicyCatalog([]TierPolicy{{
		Tier: PlanFree, AlertThreshold: 60,
		Features: FeatureEntitlements{UsageExport: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := ExportUsageJSON(ctx, store, "tenant-private-123", "2026-11", plan, catalog)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if strings.Contains(text, "tenant-private-123") || strings.Contains(text, "price") || strings.Contains(text, "blocked") {
		t.Fatalf("export contains private or unsupported billing fields: %s", text)
	}
	var report UsageExport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != "fraudguard.usage.v2" || report.Period != "2026-11" ||
		report.Usage.Requests != 1 || !report.Usage.ModelBreakdownComplete ||
		len(report.Usage.Models) != 1 || report.Usage.Models[0].Provider != "anthropic" ||
		report.Usage.Models[0].Model != "claude-sonnet-5-5" ||
		report.Plan == nil || report.Plan.Tier != PlanFree {
		t.Fatalf("unexpected export: %+v", report)
	}
	noExportCatalog, err := NewTierPolicyCatalog([]TierPolicy{{Tier: PlanFree, AlertThreshold: 60}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildUsageExport(ctx, store, "tenant-private-123", "2026-11", plan, noExportCatalog); err == nil {
		t.Fatal("usage export succeeded without the tenant's export entitlement")
	}
}

func TestRedisAuditLogIsAppendOnlyAndTamperEvident(t *testing.T) {
	store, server := newTestRedisProductStore(t)
	ctx := context.Background()
	event := AuditEvent{
		ID: "audit-event-0001", OccurredAt: time.Now().UTC(), Kind: "risk_finding",
		Score: 80, Threshold: 60, Alert: true,
		Signals: []Signal{{Name: "rapid_sequence", Reason: "request interval changed", Weight: 30}},
	}
	record, err := store.AppendAudit(ctx, "raw-tenant-for-test", event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendAudit(ctx, "raw-tenant-for-test", event); !errors.Is(err, ErrDuplicateAuditEvent) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if strings.Contains(string(mustJSON(t, record)), "raw-tenant-for-test") {
		t.Fatal("raw tenant ID leaked into audit record")
	}
	records, err := store.ListAudit(ctx, "raw-tenant-for-test", 10)
	if err != nil || len(records) != 1 {
		t.Fatalf("audit read: len=%d err=%v", len(records), err)
	}
	if records[0].Event.Kind != "risk_finding" || records[0].Event.Score != 80 {
		t.Fatalf("unexpected audit record: %+v", records[0])
	}

	var auditStream string
	for _, key := range server.Keys() {
		if strings.HasSuffix(key, ":audit") {
			auditStream = key
			break
		}
	}
	if auditStream == "" {
		t.Fatal("expected an audit stream")
	}
	entry, err := store.client.XRevRangeN(ctx, auditStream, "+", "-", 1).Result()
	if err != nil || len(entry) != 1 {
		t.Fatalf("read raw audit entry: %v", err)
	}
	if err := store.client.XDel(ctx, auditStream, entry[0].ID).Err(); err != nil {
		t.Fatal(err)
	}
	_, err = store.client.XAdd(ctx, &redis.XAddArgs{
		Stream: auditStream, Values: map[string]any{"event": `{"kind":"tampered"}`, "signature": "invalid"},
	}).Result()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListAudit(ctx, "raw-tenant-for-test", 10); err == nil {
		t.Fatal("tampered audit record was accepted")
	}
}

func TestWebhookTargetMustBeHTTPSAndAllowlisted(t *testing.T) {
	if _, err := validateWebhookTarget(WebhookTarget{
		Endpoint: "http://example.com/hook", SigningKey: []byte(strings.Repeat("k", 32)), AllowedHosts: []string{"example.com"},
	}); err == nil {
		t.Fatal("insecure HTTP endpoint was accepted")
	}
	if _, err := validateWebhookTarget(WebhookTarget{
		Endpoint: "https://example.com/hook", SigningKey: []byte(strings.Repeat("k", 32)), AllowedHosts: []string{"elsewhere.example"},
	}); err == nil {
		t.Fatal("non-allowlisted host was accepted")
	}
	if _, err := validateWebhookTarget(WebhookTarget{
		Endpoint: "https://user:pass@example.com/hook", SigningKey: []byte(strings.Repeat("k", 32)), AllowedHosts: []string{"example.com"},
	}); err == nil {
		t.Fatal("URL user information was accepted")
	}
}

func TestHTTPWebhookSendsSignedPayloadFreeAuditRecord(t *testing.T) {
	signingKey := []byte(strings.Repeat("webhook-test-key-", 3))
	var body []byte
	var signature string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		signature = r.Header.Get("X-FraudGuard-Signature")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	sender, err := NewHTTPWebhookSender(WebhookTargetResolverFunc(
		func(_ context.Context, _ string) (WebhookTarget, bool, error) {
			return WebhookTarget{
				Endpoint:       server.URL,
				SigningKey:     signingKey,
				AllowedHosts:   []string{"127.0.0.1"},
				AllowPrivateIP: true,
			}, true, nil
		},
	), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	sender.client = server.Client()
	record := AuditRecord{
		TenantRef: "hmac-tenant-reference",
		Event: AuditEvent{
			ID: "webhook-event-0001", OccurredAt: time.Now().UTC(),
			Kind: "risk_finding", Score: 75, Threshold: 60, Alert: true,
			Signals: []Signal{{Name: "new_model", Reason: "model is outside recent history", Weight: 25}},
		},
	}
	if err := sender.Send(context.Background(), "raw-tenant-secret", record); err != nil {
		t.Fatal(err)
	}
	if signature != "sha256="+webhookSignature(signingKey, body) {
		t.Fatal("webhook HMAC header does not match the delivered JSON body")
	}
	if bytes.Contains(body, []byte("raw-tenant-secret")) {
		t.Fatal("webhook body leaked the raw tenant ID")
	}
	var decoded AuditRecord
	if err := json.Unmarshal(body, &decoded); err != nil || decoded.Event.ID != record.Event.ID {
		t.Fatalf("unexpected webhook body: %+v err=%v", decoded, err)
	}
}

type testPlanResolver func(context.Context, string) (PlanTier, error)

func (f testPlanResolver) ResolveTenantTier(ctx context.Context, tenantID string) (PlanTier, error) {
	return f(ctx, tenantID)
}

type memoryUsageLedger struct {
	mu     sync.Mutex
	events map[string]UsageEvent
}

func (m *memoryUsageLedger) RecordUsage(_ context.Context, event UsageEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.events == nil {
		m.events = make(map[string]UsageEvent)
	}
	m.events[event.EventID] = event
	return nil
}

func (m *memoryUsageLedger) UsageForMonth(_ context.Context, tenantID, month string) (UsageTotals, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	totals := UsageTotals{TenantRef: "tenant-hash", Month: month}
	for _, event := range m.events {
		if event.TenantID != tenantID || event.OccurredAt.UTC().Format("2006-01") != month {
			continue
		}
		totals.Requests++
		totals.PromptTokens += event.PromptTokens
		totals.CompletionTokens += event.CompletionTokens
		totals.FraudChecks++
		totals.FraudCheckMicros += event.FraudCheckLatency.Microseconds()
		totals.Findings += event.FindingCount
		totals.Alerts += event.AlertCount
	}
	return totals, nil
}

type memoryAuditLog struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (m *memoryAuditLog) AppendAudit(_ context.Context, _ string, event AuditEvent) (AuditRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.events {
		if existing.ID == event.ID {
			return AuditRecord{}, ErrDuplicateAuditEvent
		}
	}
	m.events = append(m.events, event)
	return AuditRecord{TenantRef: "tenant-hash", Event: event, Signature: "test-signature"}, nil
}

func (m *memoryAuditLog) ListAudit(context.Context, string, int64) ([]AuditRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]AuditRecord, 0, len(m.events))
	for _, event := range m.events {
		out = append(out, AuditRecord{TenantRef: "tenant-hash", Event: event, Signature: "test-signature"})
	}
	return out, nil
}

type testWebhookSender struct {
	mu      sync.Mutex
	records []AuditRecord
}

func (w *testWebhookSender) Send(_ context.Context, _ string, record AuditRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.records = append(w.records, record)
	return nil
}

func TestProductRuntimeMetersAuditsAndAlertsWithoutBlocking(t *testing.T) {
	ledger := &memoryUsageLedger{}
	audit := &memoryAuditLog{}
	webhooks := &testWebhookSender{}
	policies, err := NewTierPolicyCatalog([]TierPolicy{{
		Tier: PlanPro, MonthlyRequestLimit: 1, AlertThreshold: 50, WarningPercent: 80,
		Features: FeatureEntitlements{UsageExport: true, WebhookAlerts: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewProductRuntime(ProductServices{
		TenantResolver: func(*schemas.BifrostContext) string { return "trusted-tenant" },
		UsageLedger:    ledger, PlanResolver: testPlanResolver(func(context.Context, string) (PlanTier, error) { return PlanPro, nil }),
		Policies: policies, AuditLog: audit, Webhooks: webhooks, QueueSize: 8, Workers: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	now := time.Now().UTC()
	if !runtime.SubmitUsage(UsageEvent{
		EventID: "product-event-0001", TenantID: "trusted-tenant", OccurredAt: now,
		PromptTokens: 40, CompletionTokens: 20, FraudCheckLatency: time.Millisecond,
		FindingCount: 1, AlertCount: 1,
	}) {
		t.Fatal("usage event was not queued")
	}
	if !runtime.SubmitFinding("trusted-tenant", "product-event-0001:finding", Finding{
		Timestamp: now, Score: 80, Threshold: 60, Alert: true,
		Signals: []Signal{{Name: "new_model", Reason: "new model observed", Weight: 25}},
	}) {
		t.Fatal("finding was not queued")
	}
	waitFor(t, func() bool {
		audit.mu.Lock()
		defer audit.mu.Unlock()
		return len(audit.events) >= 2
	})
	waitFor(t, func() bool {
		webhooks.mu.Lock()
		defer webhooks.mu.Unlock()
		return len(webhooks.records) > 0
	})
	webhooks.mu.Lock()
	webhookCount := len(webhooks.records)
	webhooks.mu.Unlock()
	if webhookCount == 0 {
		t.Fatal("expected webhook notification for alert or plan overage")
	}
	totals, err := ledger.UsageForMonth(context.Background(), "trusted-tenant", now.Format("2006-01"))
	if err != nil || totals.Requests != 1 {
		t.Fatalf("usage ledger totals=%+v err=%v", totals, err)
	}
	if metrics := runtime.Metrics(); metrics.EventsDropped != 0 || metrics.MeteringFailed != 0 {
		t.Fatalf("unexpected product metrics: %+v", metrics)
	}
}

func TestNewProductRuntimeRequiresTrustedTenantAndConsistentPlanConfig(t *testing.T) {
	if _, err := NewProductRuntime(ProductServices{UsageLedger: &memoryUsageLedger{}}); err == nil {
		t.Fatal("missing tenant resolver was accepted")
	}
	if _, err := NewProductRuntime(ProductServices{
		TenantResolver: func(*schemas.BifrostContext) string { return "tenant" },
		UsageLedger:    &memoryUsageLedger{},
		PlanResolver:   testPlanResolver(func(context.Context, string) (PlanTier, error) { return PlanPro, nil }),
	}); err == nil {
		t.Fatal("plan resolver without a catalog was accepted")
	}
}

func TestBifrostPluginMetersTrustedTenantAndPassesRequestThrough(t *testing.T) {
	store, _ := newTestRedisProductStore(t)
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyGovernanceVirtualKeyID, "internal-vk-id")
	SetTrustedTenantID(ctx, "tenant-from-auth")
	plugin, err := NewWithProductServices(Config{}, ProductServices{
		TenantResolver: TrustedTenantIDFromContext,
		UsageLedger:    store,
		AuditLog:       store,
		QueueSize:      4,
		Workers:        1,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := &schemas.BifrostRequest{}
	got, shortCircuit, err := plugin.PreLLMHook(ctx, request)
	if err != nil || shortCircuit != nil || got != request {
		t.Fatalf("PreLLMHook changed or blocked request: same=%t short=%v err=%v", got == request, shortCircuit, err)
	}
	response, bifrostErr, err := plugin.PostLLMHook(ctx, nil, nil)
	if err != nil || bifrostErr != nil || response != nil {
		t.Fatalf("PostLLMHook changed the result: response=%v error=%v hookErr=%v", response, bifrostErr, err)
	}
	if err := plugin.Cleanup(); err != nil {
		t.Fatal(err)
	}
	totals, err := store.UsageForMonth(context.Background(), "tenant-from-auth", time.Now().UTC().Format("2006-01"))
	if err != nil || totals.Requests != 1 || totals.FraudChecks != 1 {
		t.Fatalf("trusted tenant usage not recorded: totals=%+v err=%v", totals, err)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for asynchronous product processing")
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
