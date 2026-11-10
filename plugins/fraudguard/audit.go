// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrDuplicateAuditEvent = errors.New("fraudguard: duplicate audit event")

// AuditEvent is deliberately limited to sanitized operational facts. It has no
// field for prompts, payloads, credentials, raw tenant IDs, or blocked requests.
type AuditEvent struct {
	ID            string    `json:"id"`
	OccurredAt    time.Time `json:"occurred_at"`
	Kind          string    `json:"kind"`
	Score         int       `json:"score,omitempty"`
	Threshold     int       `json:"threshold,omitempty"`
	Alert         bool      `json:"alert,omitempty"`
	Signals       []Signal  `json:"signals,omitempty"`
	PlanTier      PlanTier  `json:"plan_tier,omitempty"`
	PolicyOutcome string    `json:"policy_outcome,omitempty"`
	Requests      int64     `json:"requests,omitempty"`
	TotalTokens   int64     `json:"total_tokens,omitempty"`
}

// AuditRecord contains an HMAC-signed sanitized event. The HMAC detects
// modification of retained entries; it does not prevent Redis administrators
// from deleting data, so this is tamper-evident rather than immutable storage.
type AuditRecord struct {
	TenantRef string     `json:"tenant_ref"`
	Event     AuditEvent `json:"event"`
	Signature string     `json:"signature"`
}

type AuditLog interface {
	AppendAudit(context.Context, string, AuditEvent) (AuditRecord, error)
	ListAudit(context.Context, string, int64) ([]AuditRecord, error)
}

var appendAuditScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[2]) == 1 then
  return 0
end
redis.call("XADD", KEYS[1], "*", "event", ARGV[1], "signature", ARGV[2])
redis.call("EXPIRE", KEYS[1], ARGV[3])
redis.call("SET", KEYS[2], "1", "EX", ARGV[3])
return 1
`)

func (s *RedisProductStore) AppendAudit(ctx context.Context, tenantID string, event AuditEvent) (AuditRecord, error) {
	if s == nil || s.client == nil {
		return AuditRecord{}, errors.New("fraudguard: Redis product store is not initialized")
	}
	if _, err := s.tenantRef(tenantID); err != nil {
		return AuditRecord{}, err
	}
	if err := validateAuditEvent(event); err != nil {
		return AuditRecord{}, err
	}
	tenantRef, _ := s.tenantRef(tenantID)
	event.OccurredAt = event.OccurredAt.UTC()
	event.Signals = cloneSignals(event.Signals)
	payload, err := json.Marshal(event)
	if err != nil {
		return AuditRecord{}, fmt.Errorf("fraudguard: encode audit event: %w", err)
	}
	signature := s.signAudit(tenantRef, payload)
	streamKey := s.tenantKey(tenantRef, "audit")
	dedupeKey := s.tenantKey(tenantRef, "audit-event:"+s.opaqueRef("audit-event", event.ID))
	var result int
	err = s.withRedis(ctx, func(opCtx context.Context) error {
		var runErr error
		result, runErr = appendAuditScript.Run(opCtx, s.client, []string{streamKey, dedupeKey},
			string(payload), signature, int64(s.retention.Seconds())).Int()
		return runErr
	})
	if err != nil {
		return AuditRecord{}, fmt.Errorf("fraudguard: append audit event: %w", err)
	}
	if result == 0 {
		return AuditRecord{}, ErrDuplicateAuditEvent
	}
	return AuditRecord{TenantRef: tenantRef, Event: event, Signature: signature}, nil
}

func (s *RedisProductStore) ListAudit(ctx context.Context, tenantID string, limit int64) ([]AuditRecord, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("fraudguard: Redis product store is not initialized")
	}
	tenantRef, err := s.tenantRef(tenantID)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 1000 {
		return nil, errors.New("fraudguard: audit limit must be between 1 and 1000")
	}
	var messages []redis.XMessage
	err = s.withRedis(ctx, func(opCtx context.Context) error {
		var readErr error
		messages, readErr = s.client.XRevRangeN(opCtx, s.tenantKey(tenantRef, "audit"), "+", "-", limit).Result()
		if errors.Is(readErr, redis.Nil) {
			return nil
		}
		return readErr
	})
	if err != nil {
		return nil, fmt.Errorf("fraudguard: read audit log: %w", err)
	}
	records := make([]AuditRecord, 0, len(messages))
	for _, message := range messages {
		payload := redisString(message.Values["event"])
		signature := redisString(message.Values["signature"])
		if payload == "" || signature == "" {
			return nil, errors.New("fraudguard: malformed audit record in Redis")
		}
		if !hmac.Equal([]byte(signature), []byte(s.signAudit(tenantRef, []byte(payload)))) {
			return nil, errors.New("fraudguard: audit signature verification failed")
		}
		var event AuditEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, fmt.Errorf("fraudguard: decode audit event: %w", err)
		}
		records = append(records, AuditRecord{TenantRef: tenantRef, Event: event, Signature: signature})
	}
	return records, nil
}

func (s *RedisProductStore) signAudit(tenantRef string, payload []byte) string {
	mac := hmac.New(sha256.New, s.hmacKey)
	_, _ = mac.Write([]byte("fraudguard:audit:v1:" + tenantRef + "\n"))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func validateAuditEvent(event AuditEvent) error {
	if !validEventID(event.ID) {
		return ErrInvalidEventID
	}
	if event.OccurredAt.IsZero() {
		return errors.New("fraudguard: audit timestamp is required")
	}
	if event.Kind == "" || len(event.Kind) > 64 {
		return errors.New("fraudguard: audit event kind is required and must be at most 64 characters")
	}
	for _, r := range event.Kind {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' {
			return errors.New("fraudguard: audit event kind contains unsupported characters")
		}
	}
	if event.Score < 0 || event.Score > 100 || event.Threshold < 0 || event.Threshold > 100 {
		return errors.New("fraudguard: audit score and threshold must be between 0 and 100")
	}
	if event.Requests < 0 || event.TotalTokens < 0 {
		return errors.New("fraudguard: audit usage counters cannot be negative")
	}
	for _, signal := range event.Signals {
		if signal.Name == "" || len(signal.Name) > 64 || len(signal.Reason) > 256 || signal.Weight < 0 || signal.Weight > 100 {
			return errors.New("fraudguard: invalid signal in audit event")
		}
	}
	return nil
}

func redisString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func cloneSignals(signals []Signal) []Signal {
	if signals == nil {
		return nil
	}
	return append([]Signal(nil), signals...)
}

func formatAuditTime(value time.Time) string {
	return strconv.FormatInt(value.UTC().UnixNano(), 10)
}
