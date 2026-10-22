package otelmetric

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

var reader = sdkmetric.NewManualReader()

func TestMain(m *testing.M) {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	m.Run()
}

func collect(t *testing.T) map[string]metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	out := map[string]metricdata.Metrics{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			out[m.Name] = m
		}
	}
	return out
}

func TestNormalizeRoute(t *testing.T) {
	cases := map[string]string{
		"":                       "/",
		"/":                      "/",
		"/ping":                  "/ping",
		"/v1/users/12345":        "/v1/users/:id",
		"/v1/users/12345/docs/7": "/v1/users/:id/docs/:id",
		"/v1/doc/550e8400-e29b-41d4-a716-446655440000": "/v1/doc/:id",
		"/v1/obj/507f1f77bcf86cd799439011":             "/v1/obj/:id",
		"/v1/onboarding/submit":                        "/v1/onboarding/submit",
		"/api/v2/abc":                                  "/api/v2/abc",
	}
	for in, want := range cases {
		if got := NormalizeRoute(in); got != want {
			t.Errorf("NormalizeRoute(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDurationMs_SubMillisecond(t *testing.T) {
	if got := durationMs(250 * time.Microsecond); got != 0.25 {
		t.Fatalf("got %v, want 0.25", got)
	}
}

func TestRecordHTTP_NormalizedRouteAndStatus(t *testing.T) {
	StartHTTP(context.Background(), "POST", "/v1/users/999")(201)

	ms := collect(t)
	total, ok := ms["http.server.request.total"].Data.(metricdata.Sum[int64])
	if !ok || len(total.DataPoints) == 0 {
		t.Fatalf("http.server.request.total not recorded: %+v", ms)
	}
	dp := total.DataPoints[0]
	if v, _ := dp.Attributes.Value(attribute.Key("http.route")); v.AsString() != "/v1/users/:id" {
		t.Fatalf("route not normalized: %v", v.AsString())
	}
	if v, _ := dp.Attributes.Value(attribute.Key("http.status_code")); v.AsInt64() != 201 {
		t.Fatalf("unexpected status: %v", v.AsInt64())
	}

	active, ok := ms["http.server.request.active"].Data.(metricdata.Sum[int64])
	if !ok || len(active.DataPoints) == 0 || active.DataPoints[0].Value != 0 {
		t.Fatalf("active counter should return to 0: %+v", active)
	}
}

func TestRecordFunc_RecordsHistogram(t *testing.T) {
	RecordFunc(context.Background(), "MyFunc")()

	h, ok := collect(t)["func.duration"].Data.(metricdata.Histogram[float64])
	if !ok || len(h.DataPoints) == 0 || h.DataPoints[0].Count == 0 {
		t.Fatalf("func.duration not recorded: %+v", h)
	}
}

type autoMetricUsecase struct{}

func (u *autoMetricUsecase) DoWork(ctx context.Context) {
	defer Record(ctx)()
}

func TestRecord_AutoFuncName(t *testing.T) {
	(&autoMetricUsecase{}).DoWork(context.Background())

	h, ok := collect(t)["func.duration"].Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatal("func.duration not recorded")
	}
	for _, dp := range h.DataPoints {
		if v, _ := dp.Attributes.Value(attribute.Key("func.name")); v.AsString() == "AutoMetricUsecase.DoWork" {
			return
		}
	}
	t.Fatalf("func.name AutoMetricUsecase.DoWork not found: %+v", h.DataPoints)
}
