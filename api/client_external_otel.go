package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	goravelhttp "github.com/goravel/framework/contracts/http"
	"github.com/supportapplibs/go-lib/otellog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellogapi "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/trace"
)

// externalLogMaxBodyBytes batas requestBody/responseBody pada log external call di
// OTEL. 0 = tidak dipotong, supaya response asli pihak eksternal tercatat utuh.
// Batas efektifnya ada di otellog.MaskJSON: body di atas 4 MiB tidak di-parse dan
// diganti "[omitted: body too large]" (tidak bisa di-mask dengan aman).
const externalLogMaxBodyBytes = 0

// traceExternalCall membuka span CLIENT "<METHOD> <host><path>" untuk satu call
// eksternal dan mengembalikan fungsi penutup yang mengirim log OTEL berisi
// request & response asli pihak eksternal, tertaut ke span tersebut.
//
// Parent span diambil dari otellog.GoContext bila requestCtx adalah Goravel context
// (diisi otellog.ServerMiddleware), selain itu dari requestCtx itu sendiri. Span
// sengaja TIDAK di-inject ke header req, supaya request ke pihak ketiga tidak berubah.
//
// Log hanya dikirim ke OTEL (file activity log sudah ditulis call). Header dan body
// request/response ikut dicatat; nilai sensitif di-mask (Authorization, API key,
// token, cookie, dll., lihat maskHeaders); body tidak dipotong (lihat externalLogMaxBodyBytes).
func traceExternalCall(requestCtx context.Context, req *http.Request) func(HTTPResponse, error) {
	// Dicatat sebelum call: call bisa mengganti req.URL saat mock aktif (non-prod).
	method, host, path := req.Method, req.URL.Host, req.URL.Path
	reqBody := readAndRestoreBody(req)

	spanCtx, span := otel.Tracer(os.Getenv("APP_NAME")).Start(parentContext(requestCtx), method+" "+host+path,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.url", req.URL.Scheme+"://"+host+path),
		),
	)
	start := time.Now()

	return func(resp HTTPResponse, err error) {
		elapsed := time.Since(start)
		if err != nil {
			otellog.RecordSpanError(span, err)
		} else if resp != nil {
			span.SetAttributes(attribute.Int("http.status_code", resp.GetStatusCode()))
		}
		// Header request dibaca setelah call supaya header default dari call ikut tercatat.
		emitExternalCallLog(spanCtx, method, host, path, req.Header, reqBody, resp, err, elapsed)
		span.End()
	}
}

func parentContext(requestCtx context.Context) context.Context {
	if requestCtx == nil {
		return context.Background()
	}
	if c, ok := requestCtx.(goravelhttp.Context); ok {
		return otellog.GoContext(c)
	}
	return requestCtx
}

func readAndRestoreBody(req *http.Request) []byte {
	if req.Body == nil || req.Body == http.NoBody {
		return nil
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	return body
}

// maskHeaders mengubah header menjadi JSON dengan nilai sensitif di-mask. Selain key
// yang dikenali otellog.IsSensitiveKey (authorization, apikey, token, secret, ...),
// Cookie/Set-Cookie juga di-mask karena memuat session.
func maskHeaders(h map[string]string) string {
	if len(h) == 0 {
		return ""
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		switch lk := strings.ToLower(k); {
		case lk == "cookie" || lk == "set-cookie" || otellog.IsSensitiveKey(k):
			out[k] = otellog.MaskedValue
		default:
			out[k] = v
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}

func flattenHeader(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = strings.Join(v, ",")
	}
	return out
}

func emitExternalCallLog(spanCtx context.Context, method, host, path string, reqHeader http.Header, reqBody []byte, resp HTTPResponse, callErr error, elapsed time.Duration) {
	var r otellogapi.Record
	r.SetTimestamp(time.Now())
	r.SetSeverity(otellogapi.SeverityInfo)
	r.SetBody(attribute.StringValue(fmt.Sprintf("HTTP CLIENT %s %s%s", method, host, path)))

	attrs := []attribute.KeyValue{
		attribute.String("log.type", "external_call"),
		attribute.String("app_name", os.Getenv("APP_NAME")),
		attribute.String("method", method),
		attribute.String("host", host),
		attribute.String("url", path),
		attribute.String("requestHeader", maskHeaders(flattenHeader(reqHeader))),
		attribute.String("requestBody", otellog.MaskJSON(reqBody, externalLogMaxBodyBytes)),
		attribute.Int64("responseTime", elapsed.Milliseconds()),
	}
	if sc := trace.SpanContextFromContext(spanCtx); sc.IsValid() {
		attrs = append(attrs,
			attribute.String("trace.id", sc.TraceID().String()),
			attribute.String("span.id", sc.SpanID().String()),
		)
	}
	if callErr != nil {
		r.SetSeverity(otellogapi.SeverityError)
		attrs = append(attrs, attribute.String("error", callErr.Error()))
	} else if resp != nil {
		attrs = append(attrs,
			attribute.Int("httpCode", resp.GetStatusCode()),
			attribute.String("responseHeader", maskHeaders(resp.GetHeaders())),
			attribute.String("responseBody", otellog.MaskJSON(resp.GetBody(), externalLogMaxBodyBytes)),
		)
	}
	r.AddAttributes(attrs...)

	global.GetLoggerProvider().Logger(os.Getenv("APP_NAME")).Emit(spanCtx, r)
}
