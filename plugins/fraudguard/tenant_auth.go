// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

// resolvedInternalVirtualKeyID reads only the ID that Bifrost governance has
// already resolved into request context. It never parses raw key headers,
// Authorization values, JWT claims, or provider credentials.
func resolvedInternalVirtualKeyID(ctx *schemas.BifrostContext) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(schemas.BifrostContextKeyGovernanceVirtualKeyID).(string)
	return strings.TrimSpace(id)
}
