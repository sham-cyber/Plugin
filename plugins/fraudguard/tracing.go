// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracingInstrumentationName = "github.com/maximhq/bifrost/plugins/fraudguard"

type spanRecorder interface {
	End(...trace.SpanEndOption)
	SetAttributes(...attribute.KeyValue)
	SetStatus(codes.Code, string)
}

// startFraudGuardSpan uses the host's global OpenTelemetry provider. With no
// provider configured, the OpenTelemetry API uses its no-op implementation.
func startFraudGuardSpan(ctx context.Context, name string) (context.Context, spanRecorder) {
	if ctx == nil {
		ctx = context.Background()
	}
	return otel.Tracer(tracingInstrumentationName).Start(ctx, name)
}

// markFraudGuardSpanFailure records a fixed code only; it never attaches error
// text, credentials, tenant IDs, prompts, or request payloads to a span.
func markFraudGuardSpanFailure(span spanRecorder, failureCode string) {
	if span == nil {
		return
	}
	span.SetAttributes(attribute.String("fraudguard.failure_code", failureCode))
	span.SetStatus(codes.Error, "optional product operation failed")
}
