package fraudguard

import (
	"context"
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
