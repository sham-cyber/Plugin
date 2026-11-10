// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import "testing"

func TestTraceHelpersAreSafeWithDefaultNoopProvider(t *testing.T) {
	ctx, span := startFraudGuardSpan(nil, "fraudguard.test")
	if ctx == nil || span == nil {
		t.Fatal("trace helper returned an empty context or span")
	}
	markFraudGuardSpanFailure(span, "timeout")
	span.End()
}
