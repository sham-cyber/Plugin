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
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type WebhookTarget struct {
	Endpoint       string
	SigningKey     []byte
	AllowedHosts   []string
	AllowPrivateIP bool
}

type WebhookTargetResolver interface {
	ResolveWebhookTarget(context.Context, string) (WebhookTarget, bool, error)
}

type AuditWebhookSender interface {
	Send(context.Context, string, AuditRecord) error
}

type WebhookTargetResolverFunc func(context.Context, string) (WebhookTarget, bool, error)

func (f WebhookTargetResolverFunc) ResolveWebhookTarget(ctx context.Context, tenantID string) (WebhookTarget, bool, error) {
	return f(ctx, tenantID)
}

// HTTPWebhookSender delivers signed, payload-free audit records. Targets must
// come from a trusted resolver and their hostname must be explicitly allowlisted.
type HTTPWebhookSender struct {
	Resolver WebhookTargetResolver
	Timeout  time.Duration
	client   *http.Client
}

func NewHTTPWebhookSender(resolver WebhookTargetResolver, timeout time.Duration) (*HTTPWebhookSender, error) {
	if resolver == nil {
		return nil, errors.New("fraudguard: webhook target resolver is required")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &HTTPWebhookSender{Resolver: resolver, Timeout: timeout}, nil
}

func (s *HTTPWebhookSender) Send(ctx context.Context, tenantID string, record AuditRecord) error {
	if s == nil || s.Resolver == nil {
		return errors.New("fraudguard: webhook sender is not configured")
	}
	if strings.TrimSpace(tenantID) == "" {
		return ErrInvalidTenantID
	}
	target, found, err := s.Resolver.ResolveWebhookTarget(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("fraudguard: resolve webhook target: %w", err)
	}
	if !found {
		return nil
	}
	parsed, err := validateWebhookTarget(target)
	if err != nil {
		return err
	}
	body, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("fraudguard: encode webhook event: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("fraudguard: build webhook request: %w", err)
	}
	signature, err := SignWebhookPayload(target.SigningKey, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "FraudGuard-Webhook/1")
	req.Header.Set("X-FraudGuard-Event-ID", record.Event.ID)
	req.Header.Set("X-FraudGuard-Timestamp", record.Event.OccurredAt.UTC().Format(time.RFC3339Nano))
	req.Header.Set("X-FraudGuard-Signature", signature)

	client := s.client
	if client == nil {
		client = safeWebhookClient(target, s.Timeout)
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fraudguard: send webhook: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("fraudguard: webhook returned HTTP %d", response.StatusCode)
	}
	return nil
}

func validateWebhookTarget(target WebhookTarget) (*url.URL, error) {
	if len(target.SigningKey) < 32 {
		return nil, errors.New("fraudguard: webhook signing key must be at least 32 bytes")
	}
	parsed, err := url.Parse(target.Endpoint)
	if err != nil {
		return nil, errors.New("fraudguard: invalid webhook endpoint")
	}
	if parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("fraudguard: webhook endpoint must be an HTTPS URL without user info or fragment")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	allowed := false
	for _, allowedHost := range target.AllowedHosts {
		if strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(allowedHost), "."), host) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, errors.New("fraudguard: webhook hostname is not allowlisted")
	}
	if port := parsed.Port(); port != "" {
		n, parseErr := strconv.Atoi(port)
		if parseErr != nil || n < 1 || n > 65535 {
			return nil, errors.New("fraudguard: invalid webhook port")
		}
	}
	return parsed, nil
}

func safeWebhookClient(target WebhookTarget, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, errors.New("fraudguard: malformed webhook dial address")
			}
			resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("fraudguard: resolve webhook host: %w", err)
			}
			for _, candidate := range resolved {
				addr, ok := netip.AddrFromSlice(candidate.IP)
				if !ok || (!target.AllowPrivateIP && !addr.IsGlobalUnicast()) {
					continue
				}
				if !target.AllowPrivateIP && (addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast()) {
					continue
				}
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
				if dialErr == nil {
					return conn, nil
				}
			}
			return nil, errors.New("fraudguard: webhook host resolved only to disallowed addresses")
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
