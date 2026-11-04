package fraudguard

import (
	"github.com/maximhq/bifrost/core/schemas"
)

const PluginName = "fraudguard"

type contextHandleKey struct{}
type credentialStatusContextKey struct{}

// SetTrustedCredentialStatus attaches authoritative key-state results for the
// current request. Call it only from trusted authentication or key-management
// code, never from caller-controlled request headers or payload fields.
func SetTrustedCredentialStatus(ctx *schemas.BifrostContext, status CredentialStatus) {
	if ctx != nil {
		ctx.SetValue(credentialStatusContextKey{}, status)
	}
}

// Plugin adapts the engine to Bifrost's schemas.LLMPlugin hook contract.
// Its hooks are deliberately observe-only: they never mutate or short-circuit requests.
type Plugin struct {
	engine *Engine
}

// New constructs a Bifrost plugin and its bounded in-memory detector.
func New(config Config) (*Plugin, error) {
	engine, err := NewEngine(config)
	if err != nil {
		return nil, err
	}
	return &Plugin{engine: engine}, nil
}

func (p *Plugin) GetName() string { return PluginName }

func (p *Plugin) Cleanup() error { return nil }

// PreRequestHook is required by schemas.LLMPlugin. FraudGuard does not route or mutate.
func (p *Plugin) PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

// PreLLMHook records safe request metadata and always continues processing.
func (p *Plugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if ctx == nil || req == nil {
		return req, nil, nil
	}
	// This is the authenticated internal row ID set by Bifrost governance, never
	// the raw x-bf-vk header credential.
	keyID, _ := ctx.Value(schemas.BifrostContextKeyGovernanceVirtualKeyID).(string)
	if keyID == "" {
		return req, nil, nil
	}
	credentialStatus, _ := ctx.Value(credentialStatusContextKey{}).(CredentialStatus)
	provider, model, _ := req.GetRequestFields()
	cohortID := ""
	if p.engine.cfg.EnableCrossKeyCorrelation {
		cohortID = resolvedUserID(ctx)
	}
	handle, _ := p.engine.Start(Request{
		VirtualKeyID:     keyID,
		CohortID:         cohortID,
		Provider:         string(provider),
		Model:            model,
		CredentialStatus: credentialStatus,
	})
	if handle.profileID != "" {
		ctx.SetValue(contextHandleKey{}, handle)
	}
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
	handle, ok := ctx.Value(contextHandleKey{}).(Handle)
	if !ok || handle.profileID == "" {
		return response, bifrostErr, nil
	}
	usage := response.NormalizedUsage()
	completion := Completion{Failed: bifrostErr != nil}
	if usage != nil {
		completion.InputTokens = int64(usage.PromptTokens)
		completion.OutputTokens = int64(usage.CompletionTokens)
	}
	p.engine.Complete(handle, completion)
	return response, bifrostErr, nil
}

// Engine exposes aggregate metrics and deterministic standalone scoring to operators.
func (p *Plugin) Engine() *Engine { return p.engine }

// RecordPayment accepts one trusted transaction event from a host payment
// integration. It only emits an observe-only finding; it never authorizes,
// declines, or changes a payment.
func (p *Plugin) RecordPayment(event PaymentEvent) Assessment {
	return p.engine.RecordPayment(event)
}

var _ schemas.LLMPlugin = (*Plugin)(nil)
