// Package oteljob berisi helper observability untuk job queue: mencatat dispatch
// yang gagal supaya tidak hilang diam-diam.
package oteljob

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"

	golib "github.com/supportapplibs/go-lib/log"
	"github.com/supportapplibs/go-lib/otellog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Reason untuk RecordDispatchFailure.
const (
	ReasonPanic = "panic"
	ReasonError = "error"
)

// DispatchFailedMetric adalah nama counter yang dicatat RecordDispatchFailure.
const DispatchFailedMetric = "job.dispatch.failed"

const dispatchFailedEvent = "job.dispatch.failed"

var (
	counterMu sync.Mutex
	counter   metric.Int64Counter
)

// failedCounter membuat counter sekali, setelah MeterProvider global terpasang.
// Kalau pembuatan gagal, dicoba lagi pada panggilan berikutnya (tidak di-cache).
func failedCounter() metric.Int64Counter {
	counterMu.Lock()
	defer counterMu.Unlock()
	if counter != nil {
		return counter
	}
	c, err := otel.Meter(os.Getenv("APP_NAME")).Int64Counter(
		DispatchFailedMetric,
		metric.WithDescription("Jumlah dispatch job queue yang gagal (job tidak terjadwal)"),
		metric.WithUnit("{dispatch}"),
	)
	if err != nil {
		return nil
	}
	counter = c
	return counter
}

// RecordDispatchFailure mencatat kegagalan dispatch job queue (panic maupun error
// dari Dispatch()) supaya tidak hilang diam-diam:
//
//   - log error ke runtime.log + OTEL, membawa trace.id bila ctx punya span
//   - event "job.dispatch.failed" di span aktif (span TIDAK ditandai Error —
//     proses pemanggil, mis. request approval, tetap sukses)
//   - counter metric job.dispatch.failed{job, reason} untuk alert
//
// job: nama job/queue. reason: ReasonPanic atau ReasonError. detail: error atau
// nilai panic. fields: identitas bisnis job yang gagal (mis. log.Map{"order_id": id});
// di-mask, dan TIDAK dijadikan label metric (cardinality).
func RecordDispatchFailure(ctx context.Context, job, reason string, detail any, fields ...golib.Map) {
	if ctx == nil {
		ctx = context.Background()
	}
	msg := fmt.Sprintf("%v", detail)

	logFields := golib.Map{
		"msg":    "Dispatch job gagal — job TIDAK terjadwal",
		"job":    job,
		"reason": reason,
		"error":  msg,
	}
	merged := golib.Map{}
	for _, f := range fields {
		masked, _ := otellog.MaskValue(f).(golib.Map)
		for k, v := range masked {
			merged[k] = v
			logFields[k] = v
		}
	}
	otellog.RuntimeCtx(ctx).Error(logFields)

	attrs := []attribute.KeyValue{
		attribute.String("job", job),
		attribute.String("reason", reason),
	}
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		eventAttrs := append([]attribute.KeyValue{}, attrs...)
		eventAttrs = append(eventAttrs, attribute.String("error", msg))
		keys := make([]string, 0, len(merged))
		for k := range merged {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			eventAttrs = append(eventAttrs, attribute.String(k, fmt.Sprintf("%v", merged[k])))
		}
		span.AddEvent(dispatchFailedEvent, trace.WithAttributes(eventAttrs...))
	}

	if c := failedCounter(); c != nil {
		c.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

// RecordDispatchPanic dipakai di defer untuk menggantikan
// `defer func() { _ = recover() }()`: panic tetap ditelan (perilaku lama
// dipertahankan) tetapi tercatat. Harus dipanggil LANGSUNG di defer:
//
//	defer oteljob.RecordDispatchPanic(ctx, "DispatchTransferGLDebit", log.Map{"order_id": orderId})
func RecordDispatchPanic(ctx context.Context, job string, fields ...golib.Map) {
	if r := recover(); r != nil {
		RecordDispatchFailure(ctx, job, ReasonPanic, r, fields...)
	}
}
