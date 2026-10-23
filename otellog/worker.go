package otellog

import (
	"context"

	golib "github.com/supportapplibs/go-lib/log"
)

// WorkerLogger menulis log job/worker ke worker.log lokal dan OTEL sekaligus.
type WorkerLogger struct{ ctx context.Context }

// Worker mengembalikan logger untuk job/worker. Satu panggilan menulis ke:
//
//   - worker.log lokal (golib.Worker) — perilaku lama, tetap dipertahankan
//   - runtime.log lokal + OTEL collector → Kibana (RuntimeCtx), membawa trace.id/span.id
//     dan request fields (AddFields) dari ctx, sehingga log bisa dikorelasikan dengan
//     span job/usecase/ESB
//
// Pakai ctx yang membawa span (hasil otellog.Trace di Handle), bukan ctx awal:
//
//	traceCtx, done := otellog.Trace(ctx, log.Map{"order_id": j.OrderID})
//	defer done(&err)
//	otellog.Worker(traceCtx).Info(log.Map{"message": "Job started"})
func Worker(ctx context.Context) *WorkerLogger {
	if ctx == nil {
		ctx = context.Background()
	}
	return &WorkerLogger{ctx: ctx}
}

// Info: OTEL level INFO + worker.log. otellog dipanggil lebih dulu karena ia menyalin
// map, sedangkan golib.Worker menambahkan traceID/identifier langsung ke map milik caller.
func (w *WorkerLogger) Info(m golib.Map) {
	RuntimeCtx(w.ctx).Info(m)
	golib.Worker(w.ctx).Info(m)
}

// Warning: OTEL level WARN; worker.log lokal tetap Info (golib.Worker hanya punya Info).
func (w *WorkerLogger) Warning(m golib.Map) {
	RuntimeCtx(w.ctx).Warning(m)
	golib.Worker(w.ctx).Info(m)
}

// Error: runtime.log + OTEL level ERROR (tidak ditulis ke worker.log, sama seperti
// golib.Runtime(ctx).Error yang digantikannya).
func (w *WorkerLogger) Error(m golib.Map) {
	RuntimeCtx(w.ctx).Error(m)
}
