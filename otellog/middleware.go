package otellog

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	goravelhttp "github.com/goravel/framework/contracts/http"
	golib "github.com/supportapplibs/go-lib/log"
	"github.com/supportapplibs/go-lib/otelmetric"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const otelContextKey = "otelContext"

type middlewareConfig struct {
	trustIncoming bool
	skipPaths     map[string]struct{}
	maxBodyBytes  int
	activityLog   bool
	metrics       bool
}

// MiddlewareOption mengatur perilaku ServerMiddleware.
type MiddlewareOption func(*middlewareConfig)

// WithTrustIncomingTrace menentukan apakah header traceparent/tracestate dari
// request masuk dipakai sebagai parent. Default true (service di belakang gateway).
// Set false di gateway/edge supaya client eksternal tidak bisa menyuntik trace ID
// dan keputusan sampling — gateway selalu jadi root span.
func WithTrustIncomingTrace(trust bool) MiddlewareOption {
	return func(c *middlewareConfig) { c.trustIncoming = trust }
}

// WithSkipPaths mengganti daftar path yang tidak di-trace/log/metric.
// Default: /ping, /favicon.ico.
func WithSkipPaths(paths ...string) MiddlewareOption {
	return func(c *middlewareConfig) {
		c.skipPaths = make(map[string]struct{}, len(paths))
		for _, p := range paths {
			c.skipPaths[p] = struct{}{}
		}
	}
}

// WithMaxBodyBytes batas ukuran requestBody/responseBody di activity log
// (setelah masking). Default 10000 byte.
func WithMaxBodyBytes(n int) MiddlewareOption {
	return func(c *middlewareConfig) { c.maxBodyBytes = n }
}

// WithActivityLog menyalakan/mematikan activity log OTEL. Default true.
func WithActivityLog(enabled bool) MiddlewareOption {
	return func(c *middlewareConfig) { c.activityLog = enabled }
}

// WithMetrics menyalakan/mematikan HTTP metrics (otelmetric.RecordHTTP). Default true.
func WithMetrics(enabled bool) MiddlewareOption {
	return func(c *middlewareConfig) { c.metrics = enabled }
}

// ServerMiddleware menggantikan pasangan middleware OtelMetrics + OtelActivity dan
// startSpan di controller. Untuk setiap request ia:
//
//  1. Extract traceparent dari header (bisa dimatikan, lihat WithTrustIncomingTrace)
//  2. Membuat span SERVER dan menyimpannya ke context ("otelSpan", "otelContext"),
//     sehingga otellog.Runtime / otellog.GoContext di controller langsung tersambung
//  3. Mencatat HTTP metrics (total, durasi, active)
//  4. Setelah handler selesai: set http.status_code, status Error untuk 5xx/panic,
//     lalu kirim activity log dengan body yang sudah di-mask & dipotong
//
// Pasang PALING AWAL, sebelum middleware bisnis (auth, PreRequest, dll.), supaya
// request yang ditolak middleware tersebut tetap punya trace:
//
//	facades.Route().Middleware(otellog.ServerMiddleware(), middleware.PreRequest).Group(...)
//
// Di controller, nama span bisa diperjelas:
//
//	trace.SpanFromContext(otellog.GoContext(ctx)).SetName("OrderController.Create")
func ServerMiddleware(opts ...MiddlewareOption) goravelhttp.Middleware {
	cfg := &middlewareConfig{
		trustIncoming: true,
		skipPaths:     map[string]struct{}{"/ping": {}, "/favicon.ico": {}},
		maxBodyBytes:  10000,
		activityLog:   true,
		metrics:       true,
	}
	for _, o := range opts {
		o(cfg)
	}

	return func(ctx goravelhttp.Context) {
		req := ctx.Request()
		path := req.Path()
		if _, skip := cfg.skipPaths[path]; skip {
			req.Next()
			return
		}

		method := req.Method()
		route := otelmetric.NormalizeRoute(path)

		parent := ctx.Context()
		if cfg.trustIncoming {
			parent = otel.GetTextMapPropagator().Extract(parent, propagation.HeaderCarrier(req.Headers()))
		} else {
			// Buang span context apa pun yang mungkin sudah ada → selalu root span.
			parent = trace.ContextWithSpanContext(parent, trace.SpanContext{})
		}

		goCtx, span := otel.Tracer(os.Getenv("APP_NAME")).Start(parent, method+" "+route,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", method),
				attribute.String("http.route", route),
				attribute.String("http.target", path),
			),
		)
		ctx.WithValue(spanKey, span)
		ctx.WithValue(otelContextKey, goCtx)

		var metricDone func(int)
		if cfg.metrics {
			metricDone = otelmetric.StartHTTP(ctx.Context(), method, path)
		}

		start := time.Now()
		panicked := true
		defer func() {
			status := responseStatus(ctx)
			if panicked {
				r := recover()
				span.RecordError(fmt.Errorf("panic: %v", r))
				span.SetStatus(codes.Error, "panic")
				if status < 500 {
					status = 500
				}
				finish(ctx, cfg, span, metricDone, start, status)
				panic(r)
			}
			finish(ctx, cfg, span, metricDone, start, status)
		}()

		req.Next()
		panicked = false
	}
}

func finish(ctx goravelhttp.Context, cfg *middlewareConfig, span trace.Span, metricDone func(int), start time.Time, status int) {
	span.SetAttributes(attribute.Int("http.status_code", status))
	if status >= 500 {
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
	}
	if metricDone != nil {
		metricDone(status)
	}
	if cfg.activityLog {
		emitActivity(ctx, cfg, start, status)
	}
	span.End()
}

func emitActivity(ctx goravelhttp.Context, cfg *middlewareConfig, start time.Time, status int) {
	req := ctx.Request()
	respBody := responseBody(ctx)
	Activity(ctx, golib.Map{
		"msg":          fmt.Sprintf("HTTP %s %s", req.Method(), req.Path()),
		"app_name":     os.Getenv("APP_NAME"),
		"host":         req.Host(),
		"path":         req.Path(),
		"identifier":   req.Path(),
		"clientip":     req.Header("X-Forwarded-For", req.Ip()),
		"clientapp":    req.Header("X-App", ""),
		"requestID":    req.Header("X-Request-ID", ""),
		"requestFrom":  req.Header("X-Request-From", ""),
		"requestUser":  req.Header("X-Request-User", ""),
		"requestNip":   req.Header("X-Request-Nip", ""),
		"deviceID":     req.Header("X-Device-ID", ""),
		"requestAt":    start.Format(time.RFC3339Nano),
		"requestBody":  requestBody(ctx, cfg.maxBodyBytes),
		"responseBody": MaskJSON(respBody, cfg.maxBodyBytes),
		"responseCode": extractStringField(respBody, "responseCode"),
		"responseTime": time.Since(start).Milliseconds(),
		"httpCode":     status,
	})
}

func requestBody(ctx goravelhttp.Context, maxBytes int) string {
	all := ctx.Request().All()
	if len(all) == 0 {
		return ""
	}
	b, err := json.Marshal(MaskValue(all))
	if err != nil {
		return ""
	}
	return truncate(string(b), maxBytes)
}

func responseStatus(ctx goravelhttp.Context) int {
	resp := ctx.Response()
	if resp == nil {
		return 0
	}
	if origin := resp.Origin(); origin != nil {
		return origin.Status()
	}
	return 0
}

func responseBody(ctx goravelhttp.Context) []byte {
	resp := ctx.Response()
	if resp == nil {
		return nil
	}
	origin := resp.Origin()
	if origin == nil {
		return nil
	}
	if body := origin.Body(); body != nil {
		return body.Bytes()
	}
	return nil
}

func extractStringField(body []byte, key string) string {
	if len(body) == 0 || len(body) > maxMaskParseBytes {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
