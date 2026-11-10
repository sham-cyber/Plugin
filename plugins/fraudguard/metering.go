// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
)

var (
	ErrInvalidTenantID = errors.New("fraudguard: trusted tenant ID is required")
	ErrInvalidEventID  = errors.New("fraudguard: valid event ID is required")
)

// UsageEvent contains billing-safe counters only. It never accepts prompt,
// response, key, or customer-record content.
type UsageEvent struct {
	EventID           string
	TenantID          string
	Provider          string
	Model             string
	OccurredAt        time.Time
	PromptTokens      int64
	CompletionTokens  int64
	FraudCheckLatency time.Duration
	FindingCount      int64
	AlertCount        int64
}

// ModelUsage is an authorized tenant's aggregated provider/model token volume.
// The export carries no prices and never contains request-level prompt content.
type ModelUsage struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Requests         int64  `json:"requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}

// UsageTotals is a tenant-month aggregate. TenantRef is an HMAC digest, not the
// original tenant identifier.
type UsageTotals struct {
	TenantRef              string       `json:"tenant_ref"`
	Month                  string       `json:"month"`
	Requests               int64        `json:"requests"`
	PromptTokens           int64        `json:"prompt_tokens"`
	CompletionTokens       int64        `json:"completion_tokens"`
	FraudChecks            int64        `json:"fraud_checks"`
	FraudCheckMicros       int64        `json:"fraud_check_latency_micros"`
	Findings               int64        `json:"findings"`
	Alerts                 int64        `json:"alerts"`
	ModelBreakdownComplete bool         `json:"model_breakdown_complete"`
	Models                 []ModelUsage `json:"models,omitempty"`
}

const maxUsageModelsPerTenantMonth = 64

// UsageLedger records idempotent request usage and returns monthly aggregates.
type UsageLedger interface {
	RecordUsage(context.Context, UsageEvent) error
	UsageForMonth(context.Context, string, string) (UsageTotals, error)
}

// RedisStoreOptions configures the persistent product ledger. HMACKey must be
// a stable secret shared by all replicas; never generate it at process startup.
type RedisStoreOptions struct {
	Client            redis.UniversalClient
	HMACKey           []byte
	Namespace         string
	Retention         time.Duration
	OperationTimeout  time.Duration
	Pool              RedisPoolOptions
	BreakerConfig     CircuitBreakerConfig
	FailOpenOnStartup bool
}

// RedisProductStore implements the usage ledger, tenant-plan lookup, and
// append-only audit storage on Redis. Tenant IDs are HMACed before key creation.
type RedisProductStore struct {
	client     redis.UniversalClient
	hmacKey    []byte
	namespace  string
	retention  time.Duration
	ownsClient bool
	timeout    time.Duration
	breaker    *CircuitBreaker
}

func NewRedisProductStore(options RedisStoreOptions) (*RedisProductStore, error) {
	if options.Client == nil {
		return nil, errors.New("fraudguard: Redis client is required")
	}
	if len(options.HMACKey) < 32 {
		return nil, errors.New("fraudguard: tenant HMAC key must be at least 32 bytes")
	}
	if options.Namespace == "" {
		options.Namespace = "fraudguard"
	}
	if !validNamespace(options.Namespace) {
		return nil, errors.New("fraudguard: namespace contains unsupported characters")
	}
	if options.Retention <= 0 {
		options.Retention = 400 * 24 * time.Hour
	}
	options.OperationTimeout = boundedRedisTimeout(options.OperationTimeout)
	return &RedisProductStore{
		client:    options.Client,
		hmacKey:   append([]byte(nil), options.HMACKey...),
		namespace: options.Namespace,
		retention: options.Retention,
		timeout:   options.OperationTimeout,
		breaker:   NewCircuitBreaker(options.BreakerConfig),
	}, nil
}

// NewRedisProductStoreFromURL creates and verifies a Redis connection. The URL
// should come from the host's secret manager, not a checked-in config file.
func NewRedisProductStoreFromURL(ctx context.Context, redisURL string, options RedisStoreOptions) (*RedisProductStore, error) {
	client, err := NewRedisClientFromURL(ctx, redisURL, RedisClientConfig{
		OperationTimeout:  options.OperationTimeout,
		Pool:              options.Pool,
		FailOpenOnStartup: options.FailOpenOnStartup,
	})
	if err != nil {
		return nil, err
	}
	options.Client = client
	store, err := NewRedisProductStore(options)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	store.ownsClient = true
	return store, nil
}

func boundedRedisTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 2 * time.Second
	}
	if timeout > 30*time.Second {
		return 30 * time.Second
	}
	return timeout
}

func (s *RedisProductStore) CircuitStatus() CircuitBreakerStatus {
	if s == nil {
		return CircuitBreakerStatus{State: CircuitClosed}
	}
	return s.breaker.Status()
}

// HealthCheck performs a bounded-context Redis ping for an operator probe.
// It is never called from inference hooks and does not reveal the Redis URL.
func (s *RedisProductStore) HealthCheck(ctx context.Context) error {
	if s == nil || s.client == nil {
		return errors.New("fraudguard: Redis product store is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	pingCtx, cancel := context.WithTimeout(ctx, boundedRedisTimeout(s.timeout))
	defer cancel()
	return s.client.Ping(pingCtx).Err()
}

func (s *RedisProductStore) withRedis(ctx context.Context, operation func(context.Context) error) error {
	if s == nil || s.client == nil || s.breaker == nil {
		return errors.New("fraudguard: Redis product store is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	traceCtx, span := startFraudGuardSpan(ctx, "fraudguard.redis.product_operation")
	defer span.End()
	span.SetAttributes(attribute.String("fraudguard.storage", "redis"))
	err := s.breaker.Do(traceCtx, func(parent context.Context) error {
		opCtx, cancel := context.WithTimeout(parent, s.timeout)
		defer cancel()
		return operation(opCtx)
	})
	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			markFraudGuardSpanFailure(span, "circuit_open")
		} else {
			markFraudGuardSpanFailure(span, classifyFailure(err))
		}
	}
	return err
}

func (s *RedisProductStore) Close() error {
	if s == nil || s.client == nil || !s.ownsClient {
		return nil
	}
	return s.client.Close()
}

var recordUsageScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[2]) == 1 then
  return 0
end
local priorRequests = tonumber(redis.call("HGET", KEYS[1], "requests") or "0")
if redis.call("HEXISTS", KEYS[1], "model_breakdown_incomplete") == 0 then
  redis.call("HSET", KEYS[1], "model_breakdown_incomplete", priorRequests)
end
redis.call("HINCRBY", KEYS[1], "requests", 1)
redis.call("HINCRBY", KEYS[1], "prompt_tokens", ARGV[1])
redis.call("HINCRBY", KEYS[1], "completion_tokens", ARGV[2])
redis.call("HINCRBY", KEYS[1], "fraud_checks", 1)
redis.call("HINCRBY", KEYS[1], "fraud_check_latency_micros", ARGV[3])
redis.call("HINCRBY", KEYS[1], "findings", ARGV[4])
redis.call("HINCRBY", KEYS[1], "alerts", ARGV[5])
redis.call("HSET", KEYS[1], "month", ARGV[6])
local modelRef = ARGV[8]
if modelRef == "" then
  redis.call("HINCRBY", KEYS[1], "model_breakdown_incomplete", 1)
else
  local modelPrefix = "model:" .. modelRef .. ":"
  if redis.call("HEXISTS", KEYS[1], modelPrefix .. "requests") == 1 then
    redis.call("HINCRBY", KEYS[1], modelPrefix .. "requests", 1)
    redis.call("HINCRBY", KEYS[1], modelPrefix .. "prompt_tokens", ARGV[1])
    redis.call("HINCRBY", KEYS[1], modelPrefix .. "completion_tokens", ARGV[2])
  else
    local modelCount = tonumber(redis.call("HGET", KEYS[1], "model_count") or "0")
    if modelCount < tonumber(ARGV[9]) then
      redis.call("HINCRBY", KEYS[1], "model_count", 1)
      redis.call("HSET", KEYS[1], modelPrefix .. "provider", ARGV[10])
      redis.call("HSET", KEYS[1], modelPrefix .. "model", ARGV[11])
      redis.call("HSET", KEYS[1], modelPrefix .. "requests", 1)
      redis.call("HSET", KEYS[1], modelPrefix .. "prompt_tokens", ARGV[1])
      redis.call("HSET", KEYS[1], modelPrefix .. "completion_tokens", ARGV[2])
    else
      redis.call("HINCRBY", KEYS[1], "model_breakdown_incomplete", 1)
    end
  end
end
redis.call("EXPIRE", KEYS[1], ARGV[7])
redis.call("SET", KEYS[2], "1", "EX", ARGV[7])
return 1
`)

func (s *RedisProductStore) RecordUsage(ctx context.Context, event UsageEvent) error {
	if s == nil || s.client == nil {
		return errors.New("fraudguard: Redis product store is not initialized")
	}
	if err := validateUsageEvent(event); err != nil {
		return err
	}
	tenantRef, err := s.tenantRef(event.TenantID)
	if err != nil {
		return err
	}
	provider, model := normalizeUsageModel(event.Provider, event.Model)
	modelRef := ""
	if provider != "" && model != "" {
		modelRef = s.opaqueRef("usage-model", provider+"\x00"+model)
	}
	month := event.OccurredAt.UTC().Format("2006-01")
	usageKey := s.tenantKey(tenantRef, "usage:"+month)
	dedupeKey := s.tenantKey(tenantRef, "usage-event:"+s.opaqueRef("usage-event", event.EventID))
	retentionSeconds := int64(s.retention.Seconds())
	err = s.withRedis(ctx, func(opCtx context.Context) error {
		_, runErr := recordUsageScript.Run(opCtx, s.client, []string{usageKey, dedupeKey},
			event.PromptTokens,
			event.CompletionTokens,
			event.FraudCheckLatency.Microseconds(),
			event.FindingCount,
			event.AlertCount,
			month,
			retentionSeconds,
			modelRef,
			maxUsageModelsPerTenantMonth,
			provider,
			model,
		).Result()
		return runErr
	})
	if err != nil {
		return fmt.Errorf("fraudguard: record usage in Redis: %w", err)
	}
	return nil
}

func (s *RedisProductStore) UsageForMonth(ctx context.Context, tenantID, month string) (UsageTotals, error) {
	if s == nil || s.client == nil {
		return UsageTotals{}, errors.New("fraudguard: Redis product store is not initialized")
	}
	if _, err := s.tenantRef(tenantID); err != nil {
		return UsageTotals{}, err
	}
	if !validUsageMonth(month) {
		return UsageTotals{}, errors.New("fraudguard: month must use YYYY-MM format")
	}
	tenantRef, _ := s.tenantRef(tenantID)
	var values map[string]string
	err := s.withRedis(ctx, func(opCtx context.Context) error {
		var readErr error
		values, readErr = s.client.HGetAll(opCtx, s.tenantKey(tenantRef, "usage:"+month)).Result()
		return readErr
	})
	if err != nil {
		return UsageTotals{}, fmt.Errorf("fraudguard: read usage from Redis: %w", err)
	}
	totals := UsageTotals{TenantRef: tenantRef, Month: month, ModelBreakdownComplete: true}
	fields := []struct {
		name string
		dest *int64
	}{
		{"requests", &totals.Requests},
		{"prompt_tokens", &totals.PromptTokens},
		{"completion_tokens", &totals.CompletionTokens},
		{"fraud_checks", &totals.FraudChecks},
		{"fraud_check_latency_micros", &totals.FraudCheckMicros},
		{"findings", &totals.Findings},
		{"alerts", &totals.Alerts},
	}
	for _, field := range fields {
		if raw := values[field.name]; raw != "" {
			parsed, parseErr := strconv.ParseInt(raw, 10, 64)
			if parseErr != nil || parsed < 0 {
				return UsageTotals{}, fmt.Errorf("fraudguard: invalid Redis usage field %q", field.name)
			}
			*field.dest = parsed
		}
	}
	if raw := values["model_breakdown_incomplete"]; raw != "" {
		incomplete, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || incomplete < 0 {
			return UsageTotals{}, errors.New("fraudguard: invalid Redis model-breakdown status")
		}
		totals.ModelBreakdownComplete = incomplete == 0
	} else if totals.Requests > 0 && len(values) > 0 {
		// Older ledger entries predate provider/model accounting.
		totals.ModelBreakdownComplete = false
	}
	modelsByRef := make(map[string]*ModelUsage)
	for field, raw := range values {
		if !strings.HasPrefix(field, "model:") {
			continue
		}
		parts := strings.Split(field, ":")
		if len(parts) != 3 || len(parts[1]) != 64 {
			return UsageTotals{}, errors.New("fraudguard: invalid Redis model-usage field")
		}
		modelUsage := modelsByRef[parts[1]]
		if modelUsage == nil {
			modelUsage = &ModelUsage{}
			modelsByRef[parts[1]] = modelUsage
		}
		switch parts[2] {
		case "provider":
			modelUsage.Provider = raw
		case "model":
			modelUsage.Model = raw
		case "requests", "prompt_tokens", "completion_tokens":
			count, parseErr := strconv.ParseInt(raw, 10, 64)
			if parseErr != nil || count < 0 {
				return UsageTotals{}, errors.New("fraudguard: invalid Redis model-usage counter")
			}
			switch parts[2] {
			case "requests":
				modelUsage.Requests = count
			case "prompt_tokens":
				modelUsage.PromptTokens = count
			case "completion_tokens":
				modelUsage.CompletionTokens = count
			}
		default:
			return UsageTotals{}, errors.New("fraudguard: unsupported Redis model-usage field")
		}
	}
	for _, modelUsage := range modelsByRef {
		provider, model := normalizeUsageModel(modelUsage.Provider, modelUsage.Model)
		if provider == "" || model == "" || modelUsage.Requests == 0 {
			return UsageTotals{}, errors.New("fraudguard: incomplete Redis model-usage record")
		}
		modelUsage.Provider = provider
		modelUsage.Model = model
		totals.Models = append(totals.Models, *modelUsage)
	}
	sort.Slice(totals.Models, func(i, j int) bool {
		if totals.Models[i].Provider == totals.Models[j].Provider {
			return totals.Models[i].Model < totals.Models[j].Model
		}
		return totals.Models[i].Provider < totals.Models[j].Provider
	})
	return totals, nil
}

// SetTenantTier updates the trusted plan assignment used by plan resolution.
// Call this only from an authenticated billing/admin integration.
func (s *RedisProductStore) SetTenantTier(ctx context.Context, tenantID string, tier PlanTier) error {
	if s == nil || s.client == nil {
		return errors.New("fraudguard: Redis product store is not initialized")
	}
	if _, err := s.tenantRef(tenantID); err != nil {
		return err
	}
	if !tier.Valid() {
		return fmt.Errorf("fraudguard: unknown plan tier %q", tier)
	}
	tenantRef, _ := s.tenantRef(tenantID)
	if err := s.withRedis(ctx, func(opCtx context.Context) error {
		return s.client.Set(opCtx, s.tenantKey(tenantRef, "plan"), string(tier), 0).Err()
	}); err != nil {
		return fmt.Errorf("fraudguard: store tenant plan: %w", err)
	}
	return nil
}

func (s *RedisProductStore) ResolveTenantTier(ctx context.Context, tenantID string) (PlanTier, error) {
	if s == nil || s.client == nil {
		return "", errors.New("fraudguard: Redis product store is not initialized")
	}
	tenantRef, err := s.tenantRef(tenantID)
	if err != nil {
		return "", err
	}
	var tier string
	missing := false
	err = s.withRedis(ctx, func(opCtx context.Context) error {
		var getErr error
		tier, getErr = s.client.Get(opCtx, s.tenantKey(tenantRef, "plan")).Result()
		if errors.Is(getErr, redis.Nil) {
			missing = true
			return nil
		}
		return getErr
	})
	if err != nil {
		return "", fmt.Errorf("fraudguard: resolve tenant plan: %w", err)
	}
	if missing {
		return "", fmt.Errorf("fraudguard: no plan assigned to tenant reference %s", tenantRef)
	}
	plan := PlanTier(tier)
	if !plan.Valid() {
		return "", fmt.Errorf("fraudguard: stored plan tier %q is invalid", tier)
	}
	return plan, nil
}

func (s *RedisProductStore) tenantRef(tenantID string) (string, error) {
	if strings.TrimSpace(tenantID) == "" {
		return "", ErrInvalidTenantID
	}
	return s.opaqueRef("tenant", tenantID), nil
}

func (s *RedisProductStore) opaqueRef(namespace, value string) string {
	mac := hmac.New(sha256.New, s.hmacKey)
	_, _ = mac.Write([]byte("fraudguard:" + namespace + ":v1:" + value))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *RedisProductStore) tenantKey(tenantRef, suffix string) string {
	return s.namespace + ":{" + tenantRef + "}:" + suffix
}

func validateUsageEvent(event UsageEvent) error {
	if strings.TrimSpace(event.TenantID) == "" {
		return ErrInvalidTenantID
	}
	if !validEventID(event.EventID) {
		return ErrInvalidEventID
	}
	if event.OccurredAt.IsZero() {
		return errors.New("fraudguard: usage event timestamp is required")
	}
	if event.PromptTokens < 0 || event.CompletionTokens < 0 ||
		event.FraudCheckLatency < 0 || event.FindingCount < 0 || event.AlertCount < 0 {
		return errors.New("fraudguard: usage counters cannot be negative")
	}
	if event.AlertCount > event.FindingCount {
		return errors.New("fraudguard: alert count cannot exceed finding count")
	}
	return nil
}

func validEventID(id string) bool {
	if len(id) < 8 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && r != '-' && r != '_' && r != '.' && r != ':' {
			return false
		}
	}
	return true
}

func normalizeUsageModel(provider, model string) (string, string) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if !validUsageLabel(provider) || !validUsageLabel(model) {
		return "", ""
	}
	return provider, model
}

func validUsageLabel(label string) bool {
	if label == "" || len(label) > 128 {
		return false
	}
	for _, r := range label {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && r != '.' && r != '_' && r != '-' && r != '/' && r != ':' {
			return false
		}
	}
	return true
}

func validUsageMonth(month string) bool {
	if len(month) != 7 || month[4] != '-' {
		return false
	}
	parsed, err := time.Parse("2006-01", month)
	return err == nil && parsed.Format("2006-01") == month
}

func validNamespace(namespace string) bool {
	if namespace == "" || len(namespace) > 64 {
		return false
	}
	for _, r := range namespace {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}
