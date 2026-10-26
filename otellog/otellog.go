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
	"github.com/supportapplibs/go-lib/internal/funcname"
	golib "github.com/supportapplibs/go-lib/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const spanKey = "otelSpan"

type contextKey string

const requestFieldsKey contextKey = "otelRequestFields"

// WithRequestFields menyimpan field request ke dalam Go context.
// Field ini otomatis di-merge ke setiap log call yang menggunakan context tersebut,
// tanpa perlu ditulis ulang di setiap Info/Warning/Error.
//
// Field yang sudah ada di context digabung (key yang sama ditimpa oleh fields);
// map milik context sebelumnya tidak dimodifikasi. Untuk field bisnis yang juga
// perlu muncul sebagai attribute span, pakai AddFields.
func WithRequestFields(ctx context.Context, fields golib.Map) context.Context {
	if base, ok := ctx.Value(requestFieldsKey).(golib.Map); ok && len(base) > 0 {
		merged := make(golib.Map, len(base)+len(fields))
		for k, v := range base {
			merged[k] = v
		}
		for k, v := range fields {
			merged[k] = v
		}
		fields = merged
	}
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
		r.SetBody(attribute.StringValue(msg))
	} else if msg, ok := m["message"].(string); ok {
		r.SetBody(attribute.StringValue(msg))
	}

	for k, v := range m {
		r.AddAttributes(attribute.String(k, fmt.Sprintf("%v", v)))
	}

	global.GetLoggerProvider().Logger(os.Getenv("APP_NAME")).Emit(ctx, r)
}

func (l *Logger) enrich(src golib.Map) golib.Map {
	// Salin dulu supaya map milik caller tidak ikut termodifikasi
	// (caller sering memakai ulang map yang sama untuk beberapa log call).
	// Field sensitif (password, token, nik, norek, ...) di-mask di sini sehingga
	// tidak pernah sampai ke runtime.log maupun OTEL collector.
	m := make(golib.Map, len(src)+2)
	for k, v := range src {
		if IsSensitiveKey(k) {
			m[k] = MaskedValue
		} else {
			m[k] = MaskValue(v)
		}
	}
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
	// Pakai context ber-span supaya request fields (AddFields / WithBodyFields)
	// ikut digabung ke activity log.
	l := &Logger{span: span, ctx: GoContext(ctx)}
	enriched := l.enrich(m)
	l.emit(otellog.SeverityInfo, enriched)
}

// TraceFunc membuat span + log error otomatis untuk satu fungsi, dengan nama
// span eksplisit. Untuk nama otomatis "<Struct>.<Func>" pakai Trace.
// Gunakan dengan named return error dan defer:
//
//	func (u *Usecase) MyFunc(ctx context.Context, req Req) (resp Resp, err error) {
//	    ctx, done := otellog.TraceFunc(ctx, "Usecase.MyFunc", log.Map{"key": val})
//	    defer done(&err)
//	    ...
//	}
//
// Error bisnis stderr (kode 01, 02, 03, 05, X0, X1) tidak dicatat sebagai error
// (tanpa RecordError, status Error, dan log Error); error sistem tetap dicatat.
//
// fields opsional: dipasang sebagai attribute span dan digabung ke setiap log
// yang memakai ctx hasil kembalian (termasuk log error dari done), mis.
// log.Map{"refno": req.Refno}. Nilai sensitif di-mask, nilai kosong dilewati.
func TraceFunc(ctx context.Context, name string, fields ...golib.Map) (context.Context, func(*error)) {
	return traceFunc(ctx, name, fields)
}

// Trace sama dengan TraceFunc, tetapi nama span diambil otomatis dari fungsi
// pemanggil dengan format "<Struct>.<Func>" (atau "<pkg>.<Func>" untuk fungsi
// tanpa receiver). Contoh: method (*orderNotarisUsecase).CreateOrderNotaris →
// "OrderNotarisUsecase.CreateOrderNotaris". Closure (.func1) diabaikan sehingga
// nama tetap nama method yang membungkusnya.
//
//	func (u *orderNotarisUsecase) CreateOrderNotaris(ctx context.Context, req Req) (res Res, err error) {
//	    ctx, done := otellog.Trace(ctx)
//	    defer done(&err)
//	    defer otelmetric.Record(ctx)()
//	    ...
//	}
//
// Harus dipanggil LANGSUNG dari fungsi yang ingin di-trace. Untuk wrapper
// (mis. helper startSpan di controller) pakai TraceFunc(ctx, CallerName(1)).
// Akronim pada nama struct (glDebitUsecase → GLDebitUsecase) diatur via RegisterTypeName.
func Trace(ctx context.Context, fields ...golib.Map) (context.Context, func(*error)) {
	return traceFunc(ctx, funcname.Caller(1), fields)
}

func traceFunc(ctx context.Context, name string, fields []golib.Map) (context.Context, func(*error)) {
	ctx, span := otel.Tracer(os.Getenv("APP_NAME")).Start(ctx, name)
	// fields → attribute span + request fields di ctx (ikut ke log error di bawah
	// dan ke log lain yang memakai ctx ini).
	ctx = AddFields(ctx, fields...)

	return ctx, func(errPtr *error) {
		defer span.End()
		if errPtr != nil && shouldRecordError(*errPtr) {
			markSpanError(span, *errPtr)
			RuntimeCtx(ctx).Error(golib.Map{"msg": name + ": error", "error": (*errPtr).Error()})
		}
	}
}

// TraceDB membuat DB span (kind CLIENT) bernama "db.<table>.<operation>".
// Untuk nama otomatis "<Repository>.<Func>" pakai TraceDBAuto.
// Gunakan dengan named return error dan defer:
//
//	func (r *Repo) GetList(ctx context.Context, ...) (result []Model, err error) {
//	    ctx, done := otellog.TraceDB(ctx, "nama_tabel", "SELECT")
//	    defer done(&err)
//	    ...
//	}
//
// gorm.ErrRecordNotFound (termasuk yang dibungkus %w) tidak dicatat sebagai
// error; span diberi attribute db.result="not_found".
func TraceDB(ctx context.Context, table, operation string) (context.Context, func(*error)) {
	return traceDB(ctx, "db."+table+"."+operation, table, operation)
}

// TraceDBAuto sama dengan TraceDB, tetapi nama span diambil otomatis dari fungsi
// pemanggil, mis. (*orderNotarisRepository).GetListOrderNotaris →
// "OrderNotarisRepository.GetListOrderNotaris". Tabel & operasi tetap tersimpan
// sebagai attribute db.table / db.operation.
//
//	ctx, done := otellog.TraceDBAuto(ctx, constants.TableOrderNotaris, "SELECT")
//	defer done(&err)
func TraceDBAuto(ctx context.Context, table, operation string) (context.Context, func(*error)) {
	return traceDB(ctx, funcname.Caller(1), table, operation)
}

func traceDB(ctx context.Context, name, table, operation string) (context.Context, func(*error)) {
	ctx, span := otel.Tracer(os.Getenv("APP_NAME")).Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.table", table),
			attribute.String("db.operation", operation),
		),
		trace.WithAttributes(dbSystemAttrs()...),
	)

	return ctx, func(errPtr *error) {
		defer span.End()
		if errPtr == nil || *errPtr == nil {
			return
		}
		if shouldRecordDBError(*errPtr) {
			markSpanError(span, *errPtr)
		} else if errors.Is(*errPtr, gorm.ErrRecordNotFound) {
			// hasil normal: tetap terlihat di span tanpa menjadi error
			span.SetAttributes(attribute.String("db.result", "not_found"))
		}
	}
}

// CallerName mengembalikan nama "<Struct>.<Func>" fungsi pada kedalaman stack
// skip (0 = fungsi yang memanggil CallerName, 1 = pemanggilnya, dst.). Untuk
// helper/wrapper span sendiri:
//
//	func startSpan(ctx goravelhttp.Context) (context.Context, trace.Span) {
//	    return otel.Tracer(...).Start(otellog.GoContext(ctx), otellog.CallerName(1))
//	}
func CallerName(skip int) string {
	return funcname.Caller(skip + 1)
}

// RegisterTypeName mengatur nama tampil tipe receiver untuk Trace, TraceDBAuto,
// CallerName, dan otelmetric.Record — untuk akronim yang tidak bisa ditebak:
//
//	func init() { otellog.RegisterTypeName("glDebitUsecase", "GLDebitUsecase") }
func RegisterTypeName(goName, display string) {
	funcname.RegisterTypeName(goName, display)
}

// markSpanError mencatat error dan menandai span berstatus Error. err.Error()
// hanya dipanggil bila span benar-benar direkam (OTEL aktif & ter-sample) —
// sebagian tipe error (mis. go-lib stderr) membaca config Goravel di Error().
func markSpanError(span trace.Span, err error) {
	if !span.IsRecording() {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
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
