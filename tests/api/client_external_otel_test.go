package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supportapplibs/go-lib/api"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellogapi "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type logCapture struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (c *logCapture) OnEmit(_ context.Context, r *sdklog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, r.Clone())
	return nil
}
func (c *logCapture) Enabled(context.Context, sdklog.EnabledParameters) bool { return true }
func (c *logCapture) Shutdown(context.Context) error                         { return nil }
func (c *logCapture) ForceFlush(context.Context) error                       { return nil }

type spanRecorder struct {
	mu    sync.Mutex
	ended []sdktrace.ReadOnlySpan
}

func (r *spanRecorder) OnStart(context.Context, sdktrace.ReadWriteSpan) {}
func (r *spanRecorder) OnEnd(s sdktrace.ReadOnlySpan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ended = append(r.ended, s)
}
func (r *spanRecorder) Shutdown(context.Context) error   { return nil }
func (r *spanRecorder) ForceFlush(context.Context) error { return nil }

func setupOtel(t *testing.T) (*spanRecorder, *logCapture) {
	t.Helper()
	recorder := &spanRecorder{}
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))

	capture := &logCapture{}
	prevLP := global.GetLoggerProvider()
	global.SetLoggerProvider(sdklog.NewLoggerProvider(sdklog.WithProcessor(capture)))

	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		global.SetLoggerProvider(prevLP)
	})
	return recorder, capture
}

func recordAttrs(r sdklog.Record) map[string]string {
	out := map[string]string{}
	r.WalkAttributes(func(kv attribute.KeyValue) bool {
		out[string(kv.Key)] = kv.Value.Emit()
		return true
	})
	return out
}

func clientSpan(t *testing.T, recorder *spanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range recorder.ended {
		if s.SpanKind() == trace.SpanKindClient {
			return s
		}
	}
	t.Fatal("span CLIENT tidak ditemukan")
	return nil
}

func TestHTTPClientExternal_Call_EmitsOtelLogWithExternalResponse(t *testing.T) {
	recorder, capture := setupOtel(t)

	const externalBody = `{"responseCode":"00","responseDesc":"asli dari pihak eksternal"}`
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		assert.Empty(t, r.Header.Get("traceparent"), "trace tidak boleh di-inject ke pihak ketiga")
		w.Header().Set("X-Ext-Trace", "ext-123")
		w.Header().Set("Set-Cookie", "session=rahasia")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(externalBody))
	}))
	defer server.Close()

	parentCtx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/inquiry", strings.NewReader(`{"cif":"ABC123","password":"rahasia"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	req.Header.Set("X-Api-Key", "kunci-rahasia")
	req.Header.Set("X-Request-Id", "req-1")

	resp, err := api.NewHTTPClientExternal().Call(parentCtx, req)
	parent.End()
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.GetStatusCode())
	assert.Equal(t, `{"cif":"ABC123","password":"rahasia"}`, receivedBody, "body ke pihak eksternal harus utuh")

	span := clientSpan(t, recorder)
	host := strings.TrimPrefix(server.URL, "http://")
	assert.Equal(t, "POST "+host+"/v1/inquiry", span.Name())
	assert.Equal(t, parent.SpanContext().SpanID(), span.Parent().SpanID())

	require.Len(t, capture.records, 1)
	rec := capture.records[0]
	attrs := recordAttrs(rec)
	assert.Equal(t, otellogapi.SeverityInfo, rec.Severity())
	assert.Equal(t, "external_call", attrs["log.type"])
	assert.Equal(t, host, attrs["host"])
	assert.Equal(t, "/v1/inquiry", attrs["url"])
	assert.Equal(t, "200", attrs["httpCode"])
	assert.JSONEq(t, externalBody, attrs["responseBody"])
	assert.Contains(t, attrs["requestBody"], "ABC123")
	assert.NotContains(t, attrs["requestBody"], "rahasia", "field sensitif harus di-mask")

	var reqHeader, respHeader map[string]string
	require.NoError(t, json.Unmarshal([]byte(attrs["requestHeader"]), &reqHeader))
	require.NoError(t, json.Unmarshal([]byte(attrs["responseHeader"]), &respHeader))
	assert.Equal(t, "req-1", reqHeader["X-Request-Id"])
	assert.Equal(t, "application/json", reqHeader["Accept"], "header default dari call ikut tercatat")
	assert.Equal(t, "***", reqHeader["Authorization"])
	assert.Equal(t, "***", reqHeader["X-Api-Key"])
	assert.Equal(t, "ext-123", respHeader["X-Ext-Trace"])
	assert.Equal(t, "***", respHeader["Set-Cookie"])
	assert.Equal(t, span.SpanContext().TraceID(), rec.TraceID())
	assert.Equal(t, span.SpanContext().SpanID(), rec.SpanID())
}

func TestHTTPClientExternal_Call_ConnectionErrorEmitsErrorLog(t *testing.T) {
	recorder, capture := setupOtel(t)

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close() // koneksi pasti ditolak

	req, err := http.NewRequest(http.MethodGet, url+"/down", nil)
	require.NoError(t, err)

	resp, err := api.NewHTTPClientExternal().Call(context.Background(), req)
	require.Error(t, err)
	assert.Nil(t, resp)

	span := clientSpan(t, recorder)
	assert.Equal(t, "Error", span.Status().Code.String())

	require.Len(t, capture.records, 1)
	rec := capture.records[0]
	attrs := recordAttrs(rec)
	assert.Equal(t, otellogapi.SeverityError, rec.Severity())
	assert.Contains(t, attrs["error"], "connection error")
	assert.NotContains(t, attrs, "responseBody")
	assert.NotContains(t, attrs, "responseHeader")
	assert.NotEmpty(t, attrs["requestHeader"])
}

func TestHTTPClientExternal_Call_OtelLogKeepsLargeBodyIntact(t *testing.T) {
	_, capture := setupOtel(t)

	// jauh di atas batas lama (10.000 byte), masih di bawah batas parse MaskJSON (1 MiB)
	largeBody := `{"responseCode":"00","data":"` + strings.Repeat("x", 200_000) + `"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(largeBody))
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	_, err = api.NewHTTPClientExternal().Call(context.Background(), req)
	require.NoError(t, err)

	require.Len(t, capture.records, 1)
	assert.JSONEq(t, largeBody, recordAttrs(capture.records[0])["responseBody"])
}
