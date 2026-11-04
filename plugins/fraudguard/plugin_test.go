package fraudguard

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestBifrostHooksAreObserveOnly(t *testing.T) {
	plugin, err := New(Config{WarmupRequests: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyGovernanceVirtualKeyID, "resolved-internal-id")
	request := &schemas.BifrostRequest{}
	got, shortCircuit, hookErr := plugin.PreLLMHook(ctx, request)
	if hookErr != nil || shortCircuit != nil || got != request {
		t.Fatalf("pre-hook changed or interrupted request: req=%p shortCircuit=%v err=%v", got, shortCircuit, hookErr)
	}
	gotResponse, gotErr, postErr := plugin.PostLLMHook(ctx, nil, nil)
	if postErr != nil || gotResponse != nil || gotErr != nil {
		t.Fatalf("post-hook changed result: response=%v error=%v postErr=%v", gotResponse, gotErr, postErr)
	}
}

func TestTrustedCredentialStatusHookIsOptInAndObserveOnly(t *testing.T) {
	var findings []Finding
	plugin, err := New(Config{
		EnableRevokedKeySignal: true,
		OnFinding:              func(f Finding) { findings = append(findings, f) },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyGovernanceVirtualKeyID, "resolved-internal-id")
	SetTrustedCredentialStatus(ctx, CredentialStatus{Revoked: true})
	request := &schemas.BifrostRequest{}
	got, shortCircuit, hookErr := plugin.PreLLMHook(ctx, request)
	if hookErr != nil || shortCircuit != nil || got != request {
		t.Fatalf("credential signal changed or interrupted request: req=%p shortCircuit=%v err=%v", got, shortCircuit, hookErr)
	}
	if len(findings) != 1 || len(findings[0].Signals) != 1 || findings[0].Signals[0].Name != "key_revoked" {
		t.Fatalf("trusted revoked status did not produce its opt-in finding: %+v", findings)
	}
	if strings.Contains(fmt.Sprintf("%+v", findings), "resolved-internal-id") {
		t.Fatalf("credential finding leaked the internal key ID: %+v", findings)
	}
}

func TestPluginRecordPaymentReturnsFindingWithoutBlocking(t *testing.T) {
	plugin, err := New(Config{EnablePaymentVelocity: true, MaxPaymentsPerWindow: 1})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	plugin.RecordPayment(PaymentEvent{SubjectID: "trusted-subject", Timestamp: base})
	assessment := plugin.RecordPayment(PaymentEvent{SubjectID: "trusted-subject", Timestamp: base.Add(time.Second)})
	if !containsSignal(assessment.Finding, "payment_velocity") || !assessment.Finding.Alert {
		t.Fatalf("expected observe-only payment signal: %+v", assessment.Finding)
	}
}
