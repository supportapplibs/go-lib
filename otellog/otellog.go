// Package otellog adalah wrapper tipis di atas go-lib RunLogger yang
// otomatis menambahkan trace.id dan span.id ke setiap log entry,
// sekaligus mengirim log via OTEL Log SDK → Collector → Elasticsearch.
package otellog

import (
	"context"
	"errors"
	"fmt"
	"os"

	goravelhttp "github.com/goravel/framework/contracts/http"
	golib "github.com/supportapplibs/go-lib/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const spanKey = "otelSpan"

type contextKey string

const requestFieldsKey contextKey = "otelRequestFields"

// WithRequestFields menyimpan field request ke dalam Go context.
// Field ini otomatis di-merge ke setiap log call yang menggunakan context tersebut,
// tanpa perlu ditulis ulang di setiap Info/Warning/Error.
func WithRequestFields(ctx context.Context, fields golib.Map) context.Context {
	return context.WithValue(ctx, requestFieldsKey, fields)
}

type Logger struct {
	inner golib.RunLogger
	span  trace.Span
	ctx   context.Context
}

// Runtime — pakai di controller yang punya goravelhttp.Context.
// Otomatis mengambil enriched context (dengan request fields) jika sudah di-set
// oleh OtelPropagation atau OtelRequestFields middleware.
func Runtime(ctx goravelhttp.Context) *Logger {
	span, _ := ctx.Value(spanKey).(trace.Span)
	goCtx := ctx.Context()
	if enriched, ok := ctx.Value("otelContext").(context.Context); ok {
		goCtx = enriched
	}
	return &Logger{inner: golib.Runtime(ctx.Context()), span: span, ctx: goCtx}
}

// RuntimeCtx — pakai di usecase/repository yang punya context.Context.
func RuntimeCtx(ctx context.Context) *Logger {
	return &Logger{
		inner: golib.Runtime(ctx),
		span:  trace.SpanFromContext(ctx),
		ctx:   ctx,
	}
}

func (l *Logger) Info(m golib.Map) {
	enriched := l.enrich(m)
	l.inner.Info(enriched)
	l.emit(otellog.SeverityInfo, enriched)
}

func (l *Logger) Warning(m golib.Map) {
	enriched := l.enrich(m)
	l.inner.Warning(enriched)
	l.emit(otellog.SeverityWarn, enriched)
}

func (l *Logger) Error(m golib.Map) {
	enriched := l.enrich(m)
	l.inner.Error(enriched)
	l.emit(otellog.SeverityError, enriched)
}

func (l *Logger) emit(severity otellog.Severity, m golib.Map) {
	ctx := context.Background()
	if l.span != nil {
		ctx = trace.ContextWithSpan(ctx, l.span)
	}

	var r otellog.Record
	r.SetSeverity(severity)

	if msg, ok := m["msg"].(string); ok {
		r.SetBody(otellog.StringValue(msg))
	} else if msg, ok := m["message"].(string); ok {
		r.SetBody(otellog.StringValue(msg))
	}

	for k, v := range m {
		r.AddAttributes(otellog.String(k, fmt.Sprintf("%v", v)))
	}

	global.GetLoggerProvider().Logger(os.Getenv("APP_NAME")).Emit(ctx, r)
}

func (l *Logger) enrich(m golib.Map) golib.Map {
	// auto-merge request base fields dari context — field eksplisit di m tetap prioritas
	if l.ctx != nil {
		if base, ok := l.ctx.Value(requestFieldsKey).(golib.Map); ok {
			for k, v := range base {
				if _, exists := m[k]; !exists {
					m[k] = v
				}
			}
		}
	}
	if l.span == nil {
		return m
	}
	if sc := l.span.SpanContext(); sc.IsValid() {
		m["trace.id"] = sc.TraceID().String()
		m["span.id"] = sc.SpanID().String()
	}
	return m
}

// Activity emits an HTTP activity log via OTEL SDK only (does not write to runtime.log).
// Use in middleware after calling Next() to capture full request/response with real trace.id.
func Activity(ctx goravelhttp.Context, m golib.Map) {
	span, _ := ctx.Value(spanKey).(trace.Span)
	l := &Logger{span: span}
	enriched := l.enrich(m)
	l.emit(otellog.SeverityInfo, enriched)
}

// TraceFunc membuat span + log error otomatis untuk satu fungsi.
// Gunakan dengan named return error dan defer:
//
//	func (u *Usecase) MyFunc(ctx context.Context, req Req) (resp Resp, err error) {
//	    ctx, done := otellog.TraceFunc(ctx, "MyFunc", log.Map{"key": val})
//	    defer done(&err)
//	    ...
//	}
//
// fields opsional, tidak wajib diisi.
func TraceFunc(ctx context.Context, name string, fields ...golib.Map) (context.Context, func(*error)) {
	ctx, span := otel.Tracer(os.Getenv("APP_NAME")).Start(ctx, name)

	return ctx, func(errPtr *error) {
		defer span.End()
		if errPtr != nil && *errPtr != nil {
			span.RecordError(*errPtr)
			RuntimeCtx(ctx).Error(golib.Map{"msg": name + ": error", "error": (*errPtr).Error()})
		}
	}
}

// TraceDB membuat DB span untuk operasi query di repository.
// Gunakan dengan named return error dan defer:
//
//	func (r *Repo) GetList(ctx context.Context, ...) (result []Model, err error) {
//	    ctx, done := otellog.TraceDB(ctx, "nama_tabel", "SELECT")
//	    defer done(&err)
//	    ...
//	}
func TraceDB(ctx context.Context, table, operation string) (context.Context, func(*error)) {
	ctx, span := otel.Tracer(os.Getenv("APP_NAME")).Start(ctx, "db."+table+"."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
	)

	return ctx, func(errPtr *error) {
		defer span.End()
		if errPtr != nil && *errPtr != nil {
			span.RecordError(*errPtr)
		}
	}
}

// GoContext mengambil enriched Go context (dengan OTEL span) dari goravelhttp.Context.
// Gunakan di controller saat memanggil usecase agar trace menyambung.
// Fallback ke ctx.Context() jika OTEL belum di-set.
func GoContext(ctx goravelhttp.Context) context.Context {
	if enriched, ok := ctx.Value("otelContext").(context.Context); ok {
		return enriched
	}
	return ctx.Context()
}

// SetDBAttrs menyimpan db.statement ke span aktif di ctx.
// Gunakan untuk INSERT, UPDATE, DELETE, atau single SELECT.
//
//	ctx, done := otellog.TraceDB(ctx, "mst_rekanan", "UPDATE")
//	defer done(&err)
//	otellog.SetDBAttrs(ctx, query.ToRawSql().Update("status", val))
func SetDBAttrs(ctx context.Context, statement string) {
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String("db.statement", statement),
	)
}

// SetDBPaginateAttrs menyimpan db.statement + pagination + rows_returned ke span aktif di ctx.
// Panggil setelah Paginate selesai dengan stmt yang diambil sebelum eksekusi.
//
//	ctx, done := otellog.TraceDB(ctx, "mst_rekanan", "SELECT")
//	defer done(&err)
//	stmt := query.ToRawSql().Find(&list)
//	_ = query.Paginate(page, limit, &list, &total)
//	otellog.SetDBPaginateAttrs(ctx, stmt, page, limit, total)
func SetDBPaginateAttrs(ctx context.Context, statement string, page, limit int, rowsReturned int64) {
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String("db.statement", fmt.Sprintf("%s LIMIT %d OFFSET %d", statement, limit, (page-1)*limit)),
		attribute.Int("db.page", page),
		attribute.Int("db.limit", limit),
		attribute.Int64("db.rows_returned", rowsReturned),
	)
}

func Flush(ctx context.Context) error {
	var errs []error
	if tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
		if err := tp.ForceFlush(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if lp, ok := global.GetLoggerProvider().(*sdklog.LoggerProvider); ok {
		if err := lp.ForceFlush(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func EnrichMap(ctx goravelhttp.Context, m map[string]any) map[string]any {
	span, ok := ctx.Value(spanKey).(trace.Span)
	if !ok || span == nil {
		return m
	}
	if sc := span.SpanContext(); sc.IsValid() {
		m["trace.id"] = sc.TraceID().String()
		m["span.id"] = sc.SpanID().String()
	}
	return m
}
