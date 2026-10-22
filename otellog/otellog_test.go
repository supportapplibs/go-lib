package otellog

import (
	"context"
	"errors"
	"sync"
	"testing"

	golib "github.com/supportapplibs/go-lib/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// memExporter menyimpan span yang sudah selesai di memory untuk assertion.
type memExporter struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (e *memExporter) ExportSpans(_ context.Context, s []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, s...)
	return nil
}
func (e *memExporter) Shutdown(context.Context) error { return nil }

func (e *memExporter) reset() []sdktrace.ReadOnlySpan {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.spans
	e.spans = nil
	return out
}

// memLogExporter menyimpan log record OTEL di memory untuk assertion.
type memLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *memLogExporter) Export(_ context.Context, rs []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range rs {
		e.records = append(e.records, r.Clone())
	}
	return nil
}
func (e *memLogExporter) Shutdown(context.Context) error   { return nil }
func (e *memLogExporter) ForceFlush(context.Context) error { return nil }

func (e *memLogExporter) reset() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.records
	e.records = nil
	return out
}

var (
	exp    = &memExporter{}
	logExp = &memLogExporter{}
)

func TestMain(m *testing.M) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp)))
	logglobal.SetLoggerProvider(sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(logExp))))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	m.Run()
}

// golib.Off() → RuntimeCtx memakai noop logger, tidak butuh Goravel facades.
func offCtx() context.Context { return golib.Off() }

func TestTraceFunc_ErrorSetsSpanStatus(t *testing.T) {
	exp.reset()
	err := func() (err error) {
		_, done := TraceFunc(offCtx(), "Failing")
		defer done(&err)
		return errors.New("boom")
	}()
	if err == nil {
		t.Fatal("expected error")
	}

	spans := exp.reset()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spans[0].Status(); got.Code != codes.Error || got.Description != "boom" {
		t.Fatalf("unexpected status: %+v", got)
	}
}

func TestTraceFunc_SuccessKeepsStatusUnset(t *testing.T) {
	exp.reset()
	func() (err error) {
		_, done := TraceFunc(offCtx(), "Ok")
		defer done(&err)
		return nil
	}()

	spans := exp.reset()
	if len(spans) != 1 || spans[0].Status().Code != codes.Unset {
		t.Fatalf("expected 1 span with Unset status, got %+v", spans)
	}
}

func TestTraceDB_ErrorSetsSpanStatus(t *testing.T) {
	exp.reset()
	func() (err error) {
		_, done := TraceDB(offCtx(), "users", "SELECT")
		defer done(&err)
		return errors.New("db down")
	}()

	spans := exp.reset()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "db.users.SELECT" || spans[0].Status().Code != codes.Error {
		t.Fatalf("unexpected span: name=%s status=%+v", spans[0].Name(), spans[0].Status())
	}
}

func TestEnrich_DoesNotMutateCallerMap(t *testing.T) {
	ctx, span := otel.Tracer("test").Start(context.Background(), "s")
	defer span.End()
	ctx = WithRequestFields(ctx, golib.Map{"requestID": "r-1", "msg": "base"})

	l := &Logger{span: span, ctx: ctx}
	in := golib.Map{"msg": "hello"}
	out := l.enrich(in)

	if len(in) != 1 || in["msg"] != "hello" {
		t.Fatalf("caller map mutated: %+v", in)
	}
	if out["msg"] != "hello" {
		t.Fatalf("explicit field must win over request fields, got %v", out["msg"])
	}
	if out["requestID"] != "r-1" {
		t.Fatalf("request field not merged: %+v", out)
	}
	if out["trace.id"] != span.SpanContext().TraceID().String() {
		t.Fatalf("trace.id missing: %+v", out)
	}
}
