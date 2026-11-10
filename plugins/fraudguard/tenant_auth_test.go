// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestResolvedInternalVirtualKeyIDUsesOnlyBifrostContext(t *testing.T) {
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyGovernanceVirtualKeyID, "  resolved-internal-id  ")
	if got := resolvedInternalVirtualKeyID(ctx); got != "resolved-internal-id" {
		t.Fatalf("resolved internal ID = %q", got)
	}

	ctx.SetValue(schemas.BifrostContextKeyGovernanceVirtualKeyID, "")
	if got := resolvedInternalVirtualKeyID(ctx); got != "" {
		t.Fatalf("empty Bifrost identity should remain unprofiled: %q", got)
	}
	if got := resolvedInternalVirtualKeyID(nil); got != "" {
		t.Fatalf("nil context returned identity %q", got)
	}
}
