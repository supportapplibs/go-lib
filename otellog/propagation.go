package otellog

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// TraceParent mengambil header W3C traceparent dari span aktif di ctx, untuk
// diteruskan ke proses asinkron (argumen job queue, pesan Kafka, dll.). String
// kosong bila ctx tidak membawa span (mis. OTEL mati) — aman diteruskan apa adanya.
//
//	{Type: "string", Value: otellog.TraceParent(ctx)}
func TraceParent(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

// ContextWithTraceParent mengembalikan ctx yang membawa span context remote dari
// traceparent, sehingga span yang dibuat dari ctx ini (mis. otellog.Trace di
// Handle job) menjadi CHILD dari span pengirim dan berada di trace.id yang sama.
// traceparent kosong/invalid → ctx apa adanya (proses memulai trace baru).
//
//	ctx = otellog.ContextWithTraceParent(ctx, j.TraceParent)
//	ctx, done := otellog.Trace(ctx)
func ContextWithTraceParent(ctx context.Context, traceparent string) context.Context {
	if traceparent == "" {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{"traceparent": traceparent})
}
