package otellog

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestTraceParent_EmptyWithoutSpan(t *testing.T) {
	if got := TraceParent(context.Background()); got != "" {
		t.Fatalf("expected empty traceparent without span, got %q", got)
	}
	//nolint:staticcheck // nil ctx sengaja diuji: harus aman
	if got := TraceParent(nil); got != "" {
		t.Fatalf("expected empty traceparent for nil ctx, got %q", got)
	}
}

func TestTraceParent_RoundTripKeepsTraceID(t *testing.T) {
	exp.reset()

	sender, senderDone := TraceFunc(offCtx(), "Sender")
	tp := TraceParent(sender)
	if !strings.HasPrefix(tp, "00-") {
		t.Fatalf("unexpected traceparent: %q", tp)
	}
	senderSC := trace.SpanFromContext(sender).SpanContext()
	var err error
	senderDone(&err)

	// sisi penerima (proses/job lain): ctx baru + traceparent dari argumen job
	recv := ContextWithTraceParent(offCtx(), tp)
	child, childDone := TraceFunc(recv, "Receiver")
	childSC := trace.SpanFromContext(child).SpanContext()
	childDone(&err)

	if childSC.TraceID() != senderSC.TraceID() {
		t.Fatalf("trace id not continued: %s vs %s", childSC.TraceID(), senderSC.TraceID())
	}

	spans := exp.reset()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	var receiver trace.SpanContext
	for _, s := range spans {
		if s.Name() == "Receiver" {
			receiver = s.Parent()
		}
	}
	if receiver.SpanID() != senderSC.SpanID() || !receiver.IsRemote() {
		t.Fatalf("receiver must be child of sender span (remote parent), got %+v", receiver)
	}
}

func TestContextWithTraceParent_EmptyOrInvalidKeepsContext(t *testing.T) {
	base := offCtx()
	if got := ContextWithTraceParent(base, ""); got != base {
		t.Fatal("empty traceparent must return ctx as is")
	}
	child := ContextWithTraceParent(base, "not-a-traceparent")
	if trace.SpanContextFromContext(child).IsValid() {
		t.Fatal("invalid traceparent must not produce a valid span context")
	}
}
