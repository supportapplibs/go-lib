package otellog

import (
	"bytes"
	"context"
	nethttp "net/http"
	"strings"
	"sync"
	"testing"

	mockhttp "github.com/goravel/framework/mocks/http"
	"github.com/stretchr/testify/mock"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/trace"
)

type fakeReq struct {
	method  string
	path    string
	headers nethttp.Header
	body    map[string]any
	status  int
	resp    string
	next    func()
}

// newCtx membangun mock goravel Context minimal untuk ServerMiddleware.
func newCtx(t *testing.T, f fakeReq) *mockhttp.Context {
	t.Helper()
	if f.headers == nil {
		f.headers = nethttp.Header{}
	}

	var mu sync.Mutex
	values := map[any]any{}

	ctx := mockhttp.NewContext(t)
	req := mockhttp.NewContextRequest(t)
	resp := mockhttp.NewContextResponse(t)
	origin := mockhttp.NewResponseOrigin(t)

	ctx.On("Request").Return(req).Maybe()
	ctx.On("Response").Return(resp).Maybe()
	ctx.On("Context").Return(context.Background()).Maybe()
	ctx.On("WithValue", mock.Anything, mock.Anything).Run(func(a mock.Arguments) {
		mu.Lock()
		values[a.Get(0)] = a.Get(1)
		mu.Unlock()
	}).Maybe()
	ctx.On("Value", mock.Anything).Return(func(k any) any {
		mu.Lock()
		defer mu.Unlock()
		return values[k]
	}).Maybe()

	req.On("Method").Return(f.method).Maybe()
	req.On("Path").Return(f.path).Maybe()
	req.On("Headers").Return(f.headers).Maybe()
	req.On("Host").Return("localhost").Maybe()
	req.On("Ip").Return("127.0.0.1").Maybe()
	req.On("All").Return(f.body).Maybe()
	req.On("Header", mock.Anything, mock.Anything).Return(func(k string, d ...string) string {
		if v := f.headers.Get(k); v != "" {
			return v
		}
		if len(d) > 0 {
			return d[0]
		}
		return ""
	}).Maybe()
	req.On("Next").Run(func(mock.Arguments) {
		if f.next != nil {
			f.next()
		}
	}).Maybe()

	resp.On("Origin").Return(origin).Maybe()
	origin.On("Status").Return(f.status).Maybe()
	origin.On("Body").Return(bytes.NewBufferString(f.resp)).Maybe()
	return ctx
}

func attrs(r sdklog.Record) map[string]string {
	out := map[string]string{}
	r.WalkAttributes(func(kv attribute.KeyValue) bool {
		out[string(kv.Key)] = kv.Value.AsString()
		return true
	})
	return out
}

const incomingTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

func incomingHeaders() nethttp.Header {
	h := nethttp.Header{}
	h.Set("traceparent", "00-"+incomingTraceID+"-00f067aa0ba902b7-01")
	return h
}

func TestServerMiddleware_TrustedIncomingTraceAndMaskedActivity(t *testing.T) {
	exp.reset()
	logExp.reset()

	var spanInHandler trace.Span
	var ctx *mockhttp.Context
	ctx = newCtx(t, fakeReq{
		method: "POST", path: "/v1/order/123", headers: incomingHeaders(), status: 200,
		body: map[string]any{"username": "budi", "password": "rahasia", "nik": "3201"},
		resp: `{"responseCode":"00","data":{"norekNotaris":"0101","accessToken":"tok123"}}`,
		next: func() { spanInHandler = trace.SpanFromContext(GoContext(ctx)) },
	})

	ServerMiddleware()(ctx)

	spans := exp.reset()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.SpanKind() != trace.SpanKindServer || s.Name() != "POST /v1/order/:id" {
		t.Fatalf("unexpected span kind/name: %v %q", s.SpanKind(), s.Name())
	}
	if s.SpanContext().TraceID().String() != incomingTraceID {
		t.Fatalf("trace not continued from traceparent: %s", s.SpanContext().TraceID())
	}
	if spanInHandler == nil || spanInHandler.SpanContext().SpanID() != s.SpanContext().SpanID() {
		t.Fatal("GoContext(ctx) in handler must carry the server span")
	}

	recs := logExp.reset()
	if len(recs) != 1 {
		t.Fatalf("expected 1 activity log, got %d", len(recs))
	}
	a := attrs(recs[0])
	if a["trace.id"] != incomingTraceID {
		t.Fatalf("activity log missing trace.id: %v", a["trace.id"])
	}
	if strings.Contains(a["requestBody"], "rahasia") {
		t.Fatalf("requestBody credential not masked: %s", a["requestBody"])
	}
	if !strings.Contains(a["requestBody"], `"nik":"3201"`) {
		t.Fatalf("user data (nik) must be kept: %s", a["requestBody"])
	}
	if !strings.Contains(a["requestBody"], `"username":"budi"`) {
		t.Fatalf("non-sensitive field lost: %s", a["requestBody"])
	}
	if strings.Contains(a["responseBody"], "tok123") || !strings.Contains(a["responseBody"], "0101") {
		t.Fatalf("responseBody: credential masked, user data kept: %s", a["responseBody"])
	}
	if a["responseCode"] != "00" || a["httpCode"] != "200" {
		t.Fatalf("unexpected responseCode/httpCode: %v / %v", a["responseCode"], a["httpCode"])
	}
}

func TestServerMiddleware_UntrustedStartsNewTrace(t *testing.T) {
	exp.reset()
	logExp.reset()
	ctx := newCtx(t, fakeReq{method: "POST", path: "/login", headers: incomingHeaders(), status: 200})

	ServerMiddleware(WithTrustIncomingTrace(false), WithActivityLog(false))(ctx)

	spans := exp.reset()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].SpanContext().TraceID().String() == incomingTraceID || spans[0].Parent().IsValid() {
		t.Fatal("untrusted mode must start a new root trace")
	}
	if n := len(logExp.reset()); n != 0 {
		t.Fatalf("activity log disabled but got %d records", n)
	}
}

func TestServerMiddleware_5xxMarksSpanError(t *testing.T) {
	exp.reset()
	ctx := newCtx(t, fakeReq{method: "GET", path: "/v1/x", status: 503})

	ServerMiddleware(WithActivityLog(false))(ctx)

	spans := exp.reset()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatalf("5xx must set span status Error: %+v", spans)
	}
}

func TestServerMiddleware_4xxKeepsStatusUnset(t *testing.T) {
	exp.reset()
	ctx := newCtx(t, fakeReq{method: "GET", path: "/v1/x", status: 401})

	ServerMiddleware(WithActivityLog(false))(ctx)

	spans := exp.reset()
	if len(spans) != 1 || spans[0].Status().Code != codes.Unset {
		t.Fatalf("4xx must not mark span Error: %+v", spans)
	}
}

func TestServerMiddleware_PanicRecordedAndRepanicked(t *testing.T) {
	exp.reset()
	ctx := newCtx(t, fakeReq{method: "GET", path: "/v1/x", status: 200, next: func() { panic("boom") }})

	func() {
		defer func() {
			if r := recover(); r != "boom" {
				t.Fatalf("panic must be re-thrown, got %v", r)
			}
		}()
		ServerMiddleware(WithActivityLog(false))(ctx)
	}()

	spans := exp.reset()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatalf("panic must end span with Error: %+v", spans)
	}
}

func TestServerMiddleware_SkipPath(t *testing.T) {
	exp.reset()
	called := false
	ctx := newCtx(t, fakeReq{method: "GET", path: "/ping", status: 200, next: func() { called = true }})

	ServerMiddleware()(ctx)

	if !called {
		t.Fatal("Next must still be called for skipped path")
	}
	if n := len(exp.reset()); n != 0 {
		t.Fatalf("skipped path must not create span, got %d", n)
	}
}

func TestServerMiddleware_BodyFieldsOnSpanActivityAndHandlerLogs(t *testing.T) {
	exp.reset()
	logExp.reset()

	var ctx *mockhttp.Context
	ctx = newCtx(t, fakeReq{
		method: "POST", path: "/v1/order/biaya", status: 200,
		body: map[string]any{
			"refno":          "BY-1", // refno order_biaya
			"refnoOrder":     "PL-9", // = trx_order_notaris.refno
			"refno_prakarsa": "PR-3",
			"orderId":        12345,
			"password":       "rahasia",
			"list":           []any{"x"}, // non-skalar → dilewati
		},
		resp: `{"responseCode":"00"}`,
		next: func() { RuntimeCtx(GoContext(ctx)).Info(map[string]any{"msg": "in handler"}) },
	})

	ServerMiddleware(WithBodyFields(
		"refno", "refnoPrakarsa:refno_prakarsa", "refnoOrder:refno_order",
		"orderId:order_id", "password", "list", "missing",
	))(ctx)

	spans := exp.reset()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	for k, want := range map[string]string{
		"refno": "BY-1", "refno_order": "PL-9", "refno_prakarsa": "PR-3", "order_id": "12345",
	} {
		if got := strAttr(s, k); got != want {
			t.Errorf("span attr %s = %q, want %q", k, got, want)
		}
	}
	for _, k := range []string{"password", "list", "missing"} {
		if strAttr(s, k) != "" {
			t.Errorf("span attr %s must not be set", k)
		}
	}

	recs := logExp.reset()
	if len(recs) != 2 {
		t.Fatalf("expected handler log + activity log, got %d", len(recs))
	}
	for _, r := range recs {
		a := attrs(r)
		if a["refno"] != "BY-1" || a["refno_order"] != "PL-9" || a["refno_prakarsa"] != "PR-3" || a["order_id"] != "12345" {
			t.Errorf("log %q missing business fields: %v", a["msg"], a)
		}
	}
}
