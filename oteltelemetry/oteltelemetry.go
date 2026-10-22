// Package oteltelemetry menginisialisasi OpenTelemetry (trace, log, metric) untuk
// service Goravel dalam satu panggilan, menggantikan TelemetryServiceProvider yang
// sebelumnya di-copy-paste ke setiap service.
//
// Konfigurasi exporter, TLS, sampling, dan resource mengikuti environment variable
// standar OpenTelemetry, sehingga bisa diatur dari helm chart tanpa ubah kode:
//
//	OTEL_EXPORTER_OTLP_ENDPOINT   http://otel-collector:4318 (scheme http → tanpa TLS, https → TLS)
//	OTEL_EXPORTER_OTLP_HEADERS    header tambahan (mis. auth ke collector)
//	OTEL_TRACES_SAMPLER           default parentbased_always_on; mis. parentbased_traceidratio
//	OTEL_TRACES_SAMPLER_ARG       rasio sampling, mis. 0.1
//	OTEL_SERVICE_NAME             override nama service (default: APP_NAME)
//	OTEL_RESOURCE_ATTRIBUTES      mis. service.version=1.2.3,k8s.namespace.name=mks
//
// Penggunaan di provider Goravel:
//
//	func (t *TelemetryServiceProvider) Boot(app foundation.Application) {
//	    if !facades.Config().GetBool("OTEL_ENABLED", false) {
//	        return
//	    }
//	    shutdown, err := oteltelemetry.Setup(context.Background(), oteltelemetry.ConfigFromEnv())
//	    if err != nil {
//	        fmt.Printf("[telemetry] OTEL init failed: %v\n", err)
//	        return
//	    }
//	    app.Singleton(oteltelemetry.ShutdownKey, func(foundation.Application) (any, error) { return shutdown, nil })
//	}
//
// dan saat proses berhenti (SIGTERM):
//
//	oteltelemetry.Shutdown(5 * time.Second)
package oteltelemetry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ShutdownKey adalah key container Goravel yang disarankan untuk menyimpan fungsi shutdown.
const ShutdownKey = "otelShutdown"

// ShutdownFunc mem-flush dan menutup semua provider yang dibuat oleh Setup.
type ShutdownFunc func(context.Context) error

// Config untuk Setup. Field kosong memakai default.
type Config struct {
	// ServiceName dipakai sebagai service.name (di-override OTEL_SERVICE_NAME bila diset).
	ServiceName string
	// Environment dipakai sebagai deployment.environment.
	Environment string
	// MetricInterval interval export metric. Default 30s.
	MetricInterval time.Duration
	// DisableRuntimeMetrics mematikan Go runtime metrics (goroutine, heap, GC).
	DisableRuntimeMetrics bool
}

// ConfigFromEnv membaca APP_NAME dan APP_ENV dari environment proses.
func ConfigFromEnv() Config {
	return Config{
		ServiceName: getenv("APP_NAME", "unknown"),
		Environment: getenv("APP_ENV", "development"),
	}
}

var (
	mu             sync.Mutex
	activeShutdown ShutdownFunc
)

// Setup membuat TracerProvider, LoggerProvider, dan MeterProvider dengan exporter
// OTLP/HTTP, lalu memasangnya sebagai provider global beserta propagator W3C
// TraceContext + Baggage. Jika salah satu gagal dibuat, provider yang sudah
// terbentuk ditutup kembali dan error dikembalikan (global tidak diubah).
func Setup(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "unknown"
	}
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}
	if cfg.MetricInterval <= 0 {
		cfg.MetricInterval = 30 * time.Second
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, err
	}

	var shutdowns []func(context.Context) error
	cleanup := func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for i := len(shutdowns) - 1; i >= 0; i-- {
			_ = shutdowns[i](c)
		}
	}

	traceExp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	// Sampler tidak di-set eksplisit → SDK membaca OTEL_TRACES_SAMPLER(_ARG),
	// default parentbased_always_on.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	shutdowns = append(shutdowns, tp.Shutdown)

	logExp, err := otlploghttp.New(ctx)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("create OTLP log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	shutdowns = append(shutdowns, lp.Shutdown)

	metricExp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(cfg.MetricInterval))),
		sdkmetric.WithResource(res),
	)
	shutdowns = append(shutdowns, mp.Shutdown)

	if !cfg.DisableRuntimeMetrics {
		if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
			cleanup()
			return nil, fmt.Errorf("start runtime metrics: %w", err)
		}
	}

	otel.SetTracerProvider(tp)
	logglobal.SetLoggerProvider(lp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	var once sync.Once
	var shutdownErr error
	shutdown := func(ctx context.Context) error {
		once.Do(func() {
			// Urutan: trace & log dulu (data request terakhir), metric terakhir.
			var errs []error
			for _, fn := range shutdowns {
				if err := fn(ctx); err != nil {
					errs = append(errs, err)
				}
			}
			shutdownErr = errors.Join(errs...)
		})
		return shutdownErr
	}

	mu.Lock()
	activeShutdown = shutdown
	mu.Unlock()

	return shutdown, nil
}

// Shutdown menutup provider dari Setup terakhir dengan batas waktu timeout.
// Aman dipanggil walau Setup tidak pernah dipanggil (OTEL_ENABLED=false) atau
// dipanggil berkali-kali.
func Shutdown(timeout time.Duration) error {
	mu.Lock()
	fn := activeShutdown
	mu.Unlock()
	if fn == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return fn(ctx)
}

func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	// Urutan option: nilai belakangan menimpa nilai sebelumnya, sehingga
	// OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES (WithFromEnv) menang atas
	// APP_NAME / APP_ENV.
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("deployment.environment", cfg.Environment),
		),
		resource.WithFromEnv(),
	)
	if err != nil && res == nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}
	// Error parsial (mis. satu detector gagal) tidak fatal — resource tetap dipakai.
	return res, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
