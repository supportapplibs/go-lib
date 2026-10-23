package otellog

import (
	"context"
	"testing"

	golib "github.com/supportapplibs/go-lib/log"
)

func TestWorker_EmitsOTELWithTraceAndFieldsAtEachSeverity(t *testing.T) {
	exp.reset()
	logExp.reset()

	ctx, done := Trace(offCtx(), golib.Map{"order_id": "O-7", "queue": "transfer_gl_debit"})
	w := Worker(ctx)
	w.Info(golib.Map{"message": "started"})
	w.Warning(golib.Map{"message": "slow"})
	w.Error(golib.Map{"message": "failed"})
	var err error
	done(&err)

	recs := logExp.reset()
	if len(recs) != 3 {
		t.Fatalf("expected 3 OTEL logs, got %d", len(recs))
	}
	want := []string{"INFO", "WARN", "ERROR"}
	bodies := []string{"started", "slow", "failed"}
	for i, r := range recs {
		if got := r.Severity().String(); got != want[i] {
			t.Errorf("log %d severity = %s, want %s", i, got, want[i])
		}
		if got := r.Body().AsString(); got != bodies[i] {
			t.Errorf("log %d body = %q, want %q", i, got, bodies[i])
		}
		a := attrs(r)
		if a["trace.id"] == "" || a["span.id"] == "" {
			t.Errorf("log %d missing trace.id/span.id: %v", i, a)
		}
		if a["order_id"] != "O-7" || a["queue"] != "transfer_gl_debit" {
			t.Errorf("log %d missing request fields from ctx: %v", i, a)
		}
	}
}

func TestWorker_NilContextIsSafe(t *testing.T) {
	logExp.reset()
	//nolint:staticcheck // nil ctx sengaja diuji
	Worker(nil).Info(golib.Map{"message": "no ctx"})
	if n := len(logExp.reset()); n != 1 {
		t.Fatalf("expected 1 log with nil ctx, got %d", n)
	}
}

func TestWorker_DoesNotMutateCallerMapForOTEL(t *testing.T) {
	logExp.reset()
	in := golib.Map{"message": "keep"}
	Worker(context.Background()).Info(in)
	if _, ok := in["trace.id"]; ok {
		t.Fatal("trace.id must not be injected into the caller's map")
	}
}
