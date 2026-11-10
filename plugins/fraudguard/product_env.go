// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

const (
	envRedisURL      = "FRAUDGUARD_REDIS_URL"
	envTenantHMACKey = "FRAUDGUARD_TENANT_HMAC_KEY"
	envTenantID      = "FRAUDGUARD_TENANT_ID"
	envTierPolicies  = "FRAUDGUARD_TIER_POLICIES_JSON"
	envWebhookURL    = "FRAUDGUARD_WEBHOOK_URL"
	envWebhookKey    = "FRAUDGUARD_WEBHOOK_SIGNING_KEY"
	envWebhookHosts  = "FRAUDGUARD_WEBHOOK_ALLOWED_HOSTS"
	envConfigPath    = "FRAUDGUARD_CONFIG_PATH"
	envConfigPoll    = "FRAUDGUARD_CONFIG_POLL_INTERVAL"
)

// NewFromEnvironment applies documented startup settings and enables Redis
// product services only when their required configuration is complete. With
// none of the Redis settings present it returns the standard in-memory
// observe-only plugin.
func NewFromEnvironment(config Config) (*Plugin, error) {
	var err error
	config, err = ConfigFromEnvironment(config)
	if err != nil {
		return nil, err
	}
	redisURL, hasRedisURL := os.LookupEnv(envRedisURL)
	hmacKey, hasHMACKey := os.LookupEnv(envTenantHMACKey)
	tenantID, hasTenantID := os.LookupEnv(envTenantID)
	_, hasTierPolicies := os.LookupEnv(envTierPolicies)
	webhookURL, hasWebhookURL := os.LookupEnv(envWebhookURL)
	webhookKey, hasWebhookKey := os.LookupEnv(envWebhookKey)
	webhookHosts, hasWebhookHosts := os.LookupEnv(envWebhookHosts)
	configPath, hasConfigPath := os.LookupEnv(envConfigPath)
	pollSetting, hasConfigPoll := os.LookupEnv(envConfigPoll)
	if hasConfigPoll && !hasConfigPath {
		return nil, errors.New("fraudguard: FRAUDGUARD_CONFIG_POLL_INTERVAL requires FRAUDGUARD_CONFIG_PATH")
	}
	configInterval := 5 * time.Second
	if hasConfigPoll {
		var err error
		configInterval, err = time.ParseDuration(strings.TrimSpace(pollSetting))
		if err != nil || configInterval < 100*time.Millisecond || configInterval > time.Hour {
			return nil, errors.New("fraudguard: FRAUDGUARD_CONFIG_POLL_INTERVAL must be between 100ms and 1h")
		}
	}
	if hasConfigPath && strings.TrimSpace(configPath) == "" {
		return nil, errors.New("fraudguard: FRAUDGUARD_CONFIG_PATH cannot be empty")
	}

	productConfigured := hasRedisURL || hasHMACKey || hasTenantID || hasTierPolicies ||
		hasWebhookURL || hasWebhookKey || hasWebhookHosts
	if !productConfigured {
		plugin, err := New(config)
		if err != nil {
			return nil, err
		}
		return attachRuntimeConfigWatcher(plugin, configPath, hasConfigPath, configInterval)
	}
	if !hasRedisURL || !hasHMACKey || !hasTenantID {
		return nil, errors.New("fraudguard: Redis product services require FRAUDGUARD_REDIS_URL, FRAUDGUARD_TENANT_HMAC_KEY, and FRAUDGUARD_TENANT_ID")
	}
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New("fraudguard: FRAUDGUARD_TENANT_ID cannot be empty")
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, err := NewRedisProductStoreFromURL(connectCtx, redisURL, RedisStoreOptions{
		HMACKey:           []byte(hmacKey),
		FailOpenOnStartup: true,
	})
	if err != nil {
		return nil, err
	}

	services := ProductServices{UsageLedger: store, AuditLog: store}
	// Product tenant identity must be set by trusted Bifrost host code. A
	// deployment-level tenant is a safe fallback for a single-company gateway.
	services.TenantResolver = func(ctx *schemas.BifrostContext) string {
		if trusted := TrustedTenantIDFromContext(ctx); trusted != "" {
			return trusted
		}
		return strings.TrimSpace(tenantID)
	}

	if policiesJSON, ok := os.LookupEnv(envTierPolicies); ok && strings.TrimSpace(policiesJSON) != "" {
		var policies []TierPolicy
		if err := json.Unmarshal([]byte(policiesJSON), &policies); err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("fraudguard: parse %s: %w", envTierPolicies, err)
		}
		catalog, err := NewTierPolicyCatalog(policies)
		if err != nil {
			_ = store.Close()
			return nil, err
		}
		services.PlanResolver = store
		services.Policies = catalog
	}

	if hasWebhookURL || hasWebhookKey || hasWebhookHosts {
		if !hasWebhookURL || !hasWebhookKey || !hasWebhookHosts ||
			strings.TrimSpace(webhookURL) == "" || len(webhookKey) < 32 {
			_ = store.Close()
			return nil, errors.New("fraudguard: webhook configuration requires a valid URL, signing key, and explicit allowed-host list")
		}
		allowedHosts := splitNonempty(webhookHosts)
		if len(allowedHosts) == 0 {
			_ = store.Close()
			return nil, errors.New("fraudguard: webhook allowed-host list cannot be empty")
		}
		target := WebhookTarget{Endpoint: webhookURL, SigningKey: []byte(webhookKey), AllowedHosts: allowedHosts}
		if _, err := validateWebhookTarget(target); err != nil {
			_ = store.Close()
			return nil, err
		}
		resolver := WebhookTargetResolverFunc(func(_ context.Context, _ string) (WebhookTarget, bool, error) {
			return target, true, nil
		})
		staticTenant := strings.TrimSpace(tenantID)
		resolver = func(_ context.Context, resolvedTenant string) (WebhookTarget, bool, error) {
			if strings.TrimSpace(resolvedTenant) != staticTenant {
				return WebhookTarget{}, false, nil
			}
			return target, true, nil
		}
		services.Webhooks, err = NewHTTPWebhookSender(resolver, 3*time.Second)
		if err != nil {
			_ = store.Close()
			return nil, err
		}
	}

	plugin, err := NewWithProductServices(config, services)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	plugin.productCloser = store
	if services.PlanResolver != nil {
		policyCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, found, policyErr := plugin.product.ResolvePolicy(policyCtx, strings.TrimSpace(tenantID))
		cancel()
		if policyErr != nil || !found {
			// Redis plan state is optional to inference. Keep the plugin observe-only
			// and running; missing policy disables plan-gated signals until a worker
			// can resolve the assignment successfully.
			plugin.product.NotePolicyLookupFailure()
		}
	}
	return attachRuntimeConfigWatcher(plugin, configPath, hasConfigPath, configInterval)
}

func attachRuntimeConfigWatcher(plugin *Plugin, path string, enabled bool, interval time.Duration) (*Plugin, error) {
	if !enabled {
		return plugin, nil
	}
	if err := plugin.startConfigWatcher(strings.TrimSpace(path), interval); err != nil {
		_ = plugin.Cleanup()
		return nil, err
	}
	return plugin, nil
}

func splitNonempty(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
