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
	"go.opentelemetry.io/otel/trace"
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

type autoNameUsecase struct{}

func (u *autoNameUsecase) CreateThing(ctx context.Context) (err error) {
	_, done := Trace(ctx)
	defer done(&err)
	return nil
}

func (u *autoNameUsecase) WithClosure(ctx context.Context) (err error) {
	func() {
		_, done := TraceDBAuto(ctx, "mst_thing", "SELECT") // closure → tetap nama method
		done(nil)
	}()
	return nil
}

type autoNameRepository struct{}

func (r autoNameRepository) GetByID(ctx context.Context) (err error) {
	_, done := TraceDBAuto(ctx, "mst_thing", "SELECT")
	defer done(&err)
	return nil
}

func wrapper(ctx context.Context) string { return CallerName(1) }

func (u *autoNameUsecase) ViaWrapper(ctx context.Context) string { return wrapper(ctx) }

type glDebitThing struct{}

func (g *glDebitThing) Run(ctx context.Context) (err error) {
	_, done := Trace(ctx)
	defer done(&err)
	return nil
}

func TestTrace_AutoName(t *testing.T) {
	exp.reset()
	u := &autoNameUsecase{}
	_ = u.CreateThing(offCtx())
	_ = autoNameRepository{}.GetByID(offCtx())
	_ = u.WithClosure(offCtx())

	RegisterTypeName("glDebitThing", "GLDebitThing")
	_ = (&glDebitThing{}).Run(offCtx())

	spans := exp.reset()
	want := []string{
		"AutoNameUsecase.CreateThing",
		"AutoNameRepository.GetByID",
		"AutoNameUsecase.WithClosure",
		"GLDebitThing.Run",
	}
	if len(spans) != len(want) {
		t.Fatalf("expected %d spans, got %d", len(want), len(spans))
	}
	for i, w := range want {
		if spans[i].Name() != w {
			t.Errorf("span %d: got %q, want %q", i, spans[i].Name(), w)
		}
	}
	// TraceDBAuto: kind CLIENT + attribute db.table/db.operation
	db := spans[1]
	if db.SpanKind() != trace.SpanKindClient {
		t.Fatalf("db span kind = %v", db.SpanKind())
	}
	got := map[string]string{}
	for _, kv := range db.Attributes() {
		got[string(kv.Key)] = kv.Value.AsString()
	}
	if got["db.table"] != "mst_thing" || got["db.operation"] != "SELECT" {
		t.Fatalf("db attrs missing: %v", got)
	}

	if n := u.ViaWrapper(offCtx()); n != "AutoNameUsecase.ViaWrapper" {
		t.Fatalf("CallerName(1) from wrapper = %q", n)
	}
}

func TestTraceDB_ExplicitNameKeepsLegacyFormat(t *testing.T) {
	exp.reset()
	_, done := TraceDB(offCtx(), "users", "UPDATE")
	done(nil)
	if s := exp.reset(); len(s) != 1 || s[0].Name() != "db.users.UPDATE" {
		t.Fatalf("legacy TraceDB name changed: %+v", s)
	}
}

func strAttr(s sdktrace.ReadOnlySpan, key string) string {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.AsString()
		}
	}
	return ""
}

func TestTraceFunc_FieldsBecomeAttributesAndReachErrorLog(t *testing.T) {
	exp.reset()
	logExp.reset()
	refno := "PL2606NYB1"
	err := func() (err error) {
		_, done := TraceFunc(offCtx(), "WithFields", golib.Map{
			"refno":        refno,
			"refno_order":  &refno,    // pointer ke string harus di-deref
			"password":     "rahasia", // kredensial → di-mask
			"norekNotaris": "0101",    // data pengguna → apa adanya
			"empty":        "",        // dilewati
			"nil":          nil,       // dilewati
		})
		defer done(&err)
		return errors.New("data tidak ditemukan")
	}()
	if err == nil {
		t.Fatal("expected error")
	}

	spans := exp.reset()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if strAttr(s, "refno") != refno || strAttr(s, "refno_order") != refno {
		t.Fatalf("refno attributes missing: %v", s.Attributes())
	}
	if strAttr(s, "password") != MaskedValue {
		t.Fatalf("credential must be masked, got %q", strAttr(s, "password"))
	}
	if strAttr(s, "norekNotaris") != "0101" {
		t.Fatalf("user data must be kept, got %q", strAttr(s, "norekNotaris"))
	}
	if strAttr(s, "empty") != "" || strAttr(s, "nil") != "" {
		t.Fatalf("empty/nil fields must be skipped: %v", s.Attributes())
	}

	recs := logExp.reset()
	if len(recs) != 1 {
		t.Fatalf("expected 1 error log, got %d", len(recs))
	}
	if a := attrs(recs[0]); a["refno"] != refno || a["error"] != "data tidak ditemukan" {
		t.Fatalf("error log must carry fields: %v", a)
	}
}

func TestTrace_FieldsMergeWithParentFields(t *testing.T) {
	exp.reset()
	logExp.reset()
	parent := WithRequestFields(offCtx(), golib.Map{"order_id": "O-1", "refno": "old"})
	func() (err error) {
		ctx, done := Trace(parent, golib.Map{"refno": "new"})
		defer done(&err)
		RuntimeCtx(ctx).Info(golib.Map{"msg": "hello"})
		return nil
	}()

	recs := logExp.reset()
	if len(recs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(recs))
	}
	a := attrs(recs[0])
	if a["order_id"] != "O-1" || a["refno"] != "new" {
		t.Fatalf("fields not merged (child wins): %v", a)
	}
	// ctx induk tidak boleh berubah
	if got := parent.Value(requestFieldsKey).(golib.Map)["refno"]; got != "old" {
		t.Fatalf("parent fields mutated: %v", got)
	}
}

func TestNormalizeDBSystem(t *testing.T) {
	cases := map[string]string{
		"": "", "postgresql": "postgresql", "postgres": "postgresql", "PgSQL": "postgresql",
		"mysql": "mysql", "mariadb": "mariadb", "sqlserver": "mssql", "SQLite": "sqlite",
	}
	for in, want := range cases {
		if got := normalizeDBSystem(in); got != want {
			t.Errorf("normalizeDBSystem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTraceDB_SetsDBSystemFromEnvAndOverride(t *testing.T) {
	t.Setenv("DB_CONNECTION", "postgresql")
	SetDBSystem("")
	exp.reset()
	func() (err error) {
		_, done := TraceDB(offCtx(), "mst_x", "SELECT")
		defer done(&err)
		return nil
	}()
	spans := exp.reset()
	if len(spans) != 1 || strAttr(spans[0], "db.system") != "postgresql" {
		t.Fatalf("db.system from env missing: %+v", spans)
	}

	SetDBSystem("mysql")
	t.Cleanup(func() { SetDBSystem("") })
	func() (err error) {
		_, done := TraceDB(offCtx(), "mst_x", "SELECT")
		defer done(&err)
		return nil
	}()
	spans = exp.reset()
	if len(spans) != 1 || strAttr(spans[0], "db.system") != "mysql" {
		t.Fatalf("override must win over env: %+v", spans)
	}
}

func TestTraceDB_NoDBSystemWhenUnknown(t *testing.T) {
	t.Setenv("DB_CONNECTION", "")
	SetDBSystem("")
	exp.reset()
	func() (err error) {
		_, done := TraceDB(offCtx(), "mst_x", "SELECT")
		defer done(&err)
		return nil
	}()
	spans := exp.reset()
	if len(spans) != 1 || strAttr(spans[0], "db.system") != "" {
		t.Fatalf("db.system must be omitted when undetermined: %+v", spans)
	}
}
