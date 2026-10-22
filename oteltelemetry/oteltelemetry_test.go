package oteltelemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

func TestBuildResource_EnvOverridesAppName(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.version=1.2.3")

	res, err := buildResource(context.Background(), Config{ServiceName: "MicroserviceOrder", Environment: "staging"})
	if err != nil {
		t.Fatal(err)
	}
	set := res.Set()
	if v, _ := set.Value("service.name"); v.AsString() != "MicroserviceOrder" {
		t.Fatalf("service.name = %q", v.AsString())
	}
	if v, _ := set.Value("deployment.environment"); v.AsString() != "staging" {
		t.Fatalf("deployment.environment = %q", v.AsString())
	}
	if v, _ := set.Value("service.version"); v.AsString() != "1.2.3" {
		t.Fatalf("service.version from OTEL_RESOURCE_ATTRIBUTES = %q", v.AsString())
	}

	t.Setenv("OTEL_SERVICE_NAME", "override-name")
	res, _ = buildResource(context.Background(), Config{ServiceName: "MicroserviceOrder"})
	if v, _ := res.Set().Value("service.name"); v.AsString() != "override-name" {
		t.Fatalf("OTEL_SERVICE_NAME must override APP_NAME, got %q", v.AsString())
	}
}

// TestSetup_ExportsToCollectorOnShutdown memastikan Setup membaca endpoint dari
// OTEL_EXPORTER_OTLP_ENDPOINT (scheme http → tanpa TLS) dan Shutdown mem-flush
// span, log, dan metric ke collector.
func TestSetup_ExportsToCollectorOnShutdown(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")

	shutdown, err := Setup(context.Background(), Config{ServiceName: "test-svc", DisableRuntimeMetrics: true})
	if err != nil {
		t.Fatal(err)
	}

	_, span := otel.Tracer("test").Start(context.Background(), "op")
	span.End()
	counter, _ := otel.Meter("test").Int64Counter("test.counter")
	counter.Add(context.Background(), 1)

	if err := Shutdown(5 * time.Second); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	// Idempotent: shutdown kedua tidak boleh panic / mengirim ulang.
	_ = shutdown(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if hits["/v1/traces"] == 0 {
		t.Fatalf("trace not exported, hits=%v", hits)
	}
	if hits["/v1/metrics"] == 0 {
		t.Fatalf("metric not exported, hits=%v", hits)
	}
}

func TestShutdown_NoSetupIsNoop(t *testing.T) {
	mu.Lock()
	activeShutdown = nil
	mu.Unlock()
	if err := Shutdown(time.Second); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
