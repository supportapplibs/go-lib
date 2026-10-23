package oteljob

import (
	"context"
	"errors"
	"sync"
	"testing"

	golib "github.com/supportapplibs/go-lib/log"
	"github.com/supportapplibs/go-lib/otellog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	logglobal "go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type memSpans struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (e *memSpans) ExportSpans(_ context.Context, s []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, s...)
	return nil
}
func (e *memSpans) Shutdown(context.Context) error { return nil }
func (e *memSpans) reset() []sdktrace.ReadOnlySpan {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.spans
	e.spans = nil
	return out
}

type memLogs struct {
	mu   sync.Mutex
	recs []sdklog.Record
}

func (e *memLogs) Export(_ context.Context, rs []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range rs {
		e.recs = append(e.recs, r.Clone())
	}
	return nil
}
func (e *memLogs) Shutdown(context.Context) error   { return nil }
func (e *memLogs) ForceFlush(context.Context) error { return nil }
func (e *memLogs) reset() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.recs
	e.recs = nil
	return out
}

var (
	spanExp = &memSpans{}
	logExp  = &memLogs{}
	reader  = sdkmetric.NewManualReader()
)

func TestMain(m *testing.M) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExp)))
	logglobal.SetLoggerProvider(sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(logExp))))
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	m.Run()
}

// golib.Off() → RuntimeCtx memakai noop logger lokal, tidak butuh Goravel facades.
func offCtx() context.Context { return golib.Off() }

// counterPoints membaca data point job.dispatch.failed per (job, reason).
func counterPoints(t *testing.T) map[[2]string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	out := map[[2]string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, mt := range sm.Metrics {
			if mt.Name != DispatchFailedMetric {
				continue
			}
			sum := mt.Data.(metricdata.Sum[int64])
			for _, dp := range sum.DataPoints {
				job, _ := dp.Attributes.Value(attribute.Key("job"))
				reason, _ := dp.Attributes.Value(attribute.Key("reason"))
				out[[2]string{job.AsString(), reason.AsString()}] = dp.Value
			}
		}
	}
	return out
}

func attrMap(kvs []attribute.KeyValue) map[string]string {
	out := map[string]string{}
	for _, kv := range kvs {
		out[string(kv.Key)] = kv.Value.Emit()
	}
	return out
}

func TestRecordDispatchFailure_LogEventAndCounter(t *testing.T) {
	spanExp.reset()
	logExp.reset()

	var err error
	ctx, done := otellog.TraceFunc(offCtx(), "Approval")
	RecordDispatchFailure(ctx, "DispatchTransferGLDebit", ReasonError, errors.New("redis down"),
		golib.Map{"order_id": "O-1", "accessToken": "tok"})
	done(&err)

	// span: event ada, status TIDAK Error
	spans := spanExp.reset()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status().Code.String() == "Error" {
		t.Fatal("dispatch failure must not mark the caller span as Error")
	}
	var found bool
	for _, ev := range spans[0].Events() {
		if ev.Name != dispatchFailedEvent {
			continue
		}
		found = true
		a := attrMap(ev.Attributes)
		if a["job"] != "DispatchTransferGLDebit" || a["reason"] != ReasonError ||
			a["error"] != "redis down" || a["order_id"] != "O-1" {
			t.Fatalf("unexpected event attrs: %v", a)
		}
		if a["accessToken"] != otellog.MaskedValue {
			t.Fatalf("credential must be masked in event, got %q", a["accessToken"])
		}
	}
	if !found {
		t.Fatal("job.dispatch.failed event not found on span")
	}

	// log error membawa trace.id dan field
	recs := logExp.reset()
	if len(recs) != 1 {
		t.Fatalf("expected 1 error log, got %d", len(recs))
	}
	if recs[0].Severity().String() != "ERROR" {
		t.Fatalf("expected ERROR severity, got %v", recs[0].Severity())
	}
	got := map[string]string{}
	recs[0].WalkAttributes(func(kv attribute.KeyValue) bool {
		got[string(kv.Key)] = kv.Value.Emit()
		return true
	})
	if got["job"] != "DispatchTransferGLDebit" || got["order_id"] != "O-1" || got["trace.id"] == "" {
		t.Fatalf("log attrs incomplete: %v", got)
	}

	// counter: label hanya job + reason (tanpa order_id)
	if v := counterPoints(t)[[2]string{"DispatchTransferGLDebit", ReasonError}]; v != 1 {
		t.Fatalf("counter = %d, want 1", v)
	}
}

func TestRecordDispatchPanic_SwallowsAndRecords(t *testing.T) {
	spanExp.reset()
	logExp.reset()

	func() {
		defer RecordDispatchPanic(offCtx(), "DispatchPencairanNotaris", golib.Map{"order_id": "O-2"})
		panic("boom")
	}() // tidak boleh panic ke luar

	recs := logExp.reset()
	if len(recs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(recs))
	}
	if v := counterPoints(t)[[2]string{"DispatchPencairanNotaris", ReasonPanic}]; v != 1 {
		t.Fatalf("counter = %d, want 1", v)
	}
}

func TestRecordDispatchPanic_NoPanicRecordsNothing(t *testing.T) {
	logExp.reset()
	func() {
		defer RecordDispatchPanic(offCtx(), "DispatchNoop")
	}()
	if n := len(logExp.reset()); n != 0 {
		t.Fatalf("expected no log without panic, got %d", n)
	}
	if _, ok := counterPoints(t)[[2]string{"DispatchNoop", ReasonPanic}]; ok {
		t.Fatal("counter must not be incremented without panic")
	}
}

func TestRecordDispatchFailure_NilContextIsSafe(t *testing.T) {
	//nolint:staticcheck // nil ctx sengaja diuji
	RecordDispatchFailure(nil, "DispatchNilCtx", ReasonError, "x")
}
