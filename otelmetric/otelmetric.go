// Package otelmetric adalah wrapper tipis di atas OTel Metrics API yang
// menyediakan helper untuk mencatat durasi fungsi, operasi DB, dan HTTP request.
// Pola penggunaannya sengaja dibuat mirip otellog agar konsisten di semua service.
package otelmetric

import (
	"context"
	"os"
	"sync"
	"time"

	goravelhttp "github.com/goravel/framework/contracts/http"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// defaultBuckets adalah bucket histogram standar dalam ms — cocok untuk API service.
var defaultBuckets = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

var (
	histMu sync.RWMutex
	hists  = map[string]metric.Float64Histogram{}

	httpOnce     sync.Once
	httpTotal    metric.Int64Counter
	httpDuration metric.Float64Histogram
	httpActive   metric.Int64UpDownCounter
)

func serviceMeter() metric.Meter {
	return otel.Meter(os.Getenv("APP_NAME"))
}

// getHistogram mengambil atau membuat histogram dari cache internal.
// Aman untuk dipanggil concurrent — tidak membuat instrumen duplikat.
func getHistogram(name, unit, desc string) metric.Float64Histogram {
	histMu.RLock()
	if h, ok := hists[name]; ok {
		histMu.RUnlock()
		return h
	}
	histMu.RUnlock()

	histMu.Lock()
	defer histMu.Unlock()
	if h, ok := hists[name]; ok {
		return h
	}

	h, _ := serviceMeter().Float64Histogram(name,
		metric.WithUnit(unit),
		metric.WithDescription(desc),
		metric.WithExplicitBucketBoundaries(defaultBuckets...),
	)
	hists[name] = h
	return h
}

// RecordFunc mencatat durasi satu pemanggilan fungsi sebagai histogram.
// Pasangan dari otellog.TraceFunc — gunakan keduanya bersama untuk observability lengkap.
//
//	func (u *Usecase) MyFunc(ctx context.Context, req Req) (resp Resp, err error) {
//	    ctx, traceDone := otellog.TraceFunc(ctx, "MyFunc")
//	    defer otelmetric.RecordFunc(ctx, "MyFunc")()
//	    defer traceDone(&err)
//	    ...
//	}
func RecordFunc(ctx context.Context, funcName string) func() {
	hist := getHistogram("func.duration", "ms", "Function execution duration in milliseconds")
	start := time.Now()
	return func() {
		hist.Record(ctx, float64(time.Since(start).Milliseconds()),
			metric.WithAttributes(attribute.String("func.name", funcName)),
		)
	}
}

// RecordDB mencatat durasi operasi database sebagai histogram.
// Pasangan dari otellog.TraceDB — gunakan keduanya bersama.
//
//	func (r *Repo) GetList(ctx context.Context) (result []Model, err error) {
//	    ctx, traceDone := otellog.TraceDB(ctx, "mst_rekanan", "SELECT")
//	    defer otelmetric.RecordDB(ctx, "mst_rekanan", "SELECT")()
//	    defer traceDone(&err)
//	    ...
//	}
func RecordDB(ctx context.Context, table, operation string) func() {
	hist := getHistogram("db.client.operation.duration", "ms", "Database operation duration in milliseconds")
	start := time.Now()
	return func() {
		hist.Record(ctx, float64(time.Since(start).Milliseconds()),
			metric.WithAttributes(
				attribute.String("db.table", table),
				attribute.String("db.operation", operation),
			),
		)
	}
}

func initHTTP() {
	m := serviceMeter()
	httpTotal, _ = m.Int64Counter(
		"http.server.request.total",
		metric.WithDescription("Total number of HTTP requests processed"),
		metric.WithUnit("{request}"),
	)
	httpDuration, _ = m.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("HTTP server request duration in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(defaultBuckets...),
	)
	httpActive, _ = m.Int64UpDownCounter(
		"http.server.request.active",
		metric.WithDescription("Number of HTTP requests currently being processed"),
		metric.WithUnit("{request}"),
	)
}

// RecordHTTP mencatat metrics HTTP request: total, durasi (histogram), dan concurrent aktif.
// Gunakan di middleware — panggil sebelum Next(), gunakan returned func setelah Next():
//
//	func OtelMetrics(ctx goravelhttp.Context) {
//	    done := otelmetric.RecordHTTP(ctx)
//	    ctx.Request().Next()
//	    done()
//	}
func RecordHTTP(ctx goravelhttp.Context) func() {
	httpOnce.Do(initHTTP)

	method := ctx.Request().Method()
	route := ctx.Request().Path()
	start := time.Now()

	activeAttrs := metric.WithAttributes(
		attribute.String("http.method", method),
		attribute.String("http.route", route),
	)
	httpActive.Add(ctx.Context(), 1, activeAttrs)

	return func() {
		status := ctx.Response().Origin().Status()
		duration := float64(time.Since(start).Milliseconds())

		finalAttrs := metric.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.route", route),
			attribute.Int("http.status_code", status),
		)

		httpTotal.Add(ctx.Context(), 1, finalAttrs)
		httpDuration.Record(ctx.Context(), duration, finalAttrs)
		httpActive.Add(ctx.Context(), -1, activeAttrs)
	}
}
