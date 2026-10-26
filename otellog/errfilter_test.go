package otellog

import (
	"errors"
	"fmt"
	"testing"

	"github.com/supportapplibs/go-lib/stderr"
	"go.opentelemetry.io/otel/codes"
	"gorm.io/gorm"
)

func TestShouldRecordError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not found", stderr.ErrNotFound("rekanan_not_found"), false},
		{"data not found", stderr.ErrDataNotFound(), false},
		{"param", stderr.ErrParam("x"), false},
		{"invalid rule", stderr.ErrInvRule("x"), false},
		{"invalid header", stderr.ErrInvHeader("x"), false},
		{"permission", stderr.ErrPermission("x"), false},
		{"wrapped business error", fmt.Errorf("wrap: %w", stderr.ErrNotFound("x")), false},
		{"third party", stderr.ErrThirdParty("x"), true},
		{"runtime", stderr.ErrRuntime("x"), true},
		{"waiting", stderr.ErrWaiting("x"), false},
		{"unsupported", stderr.ErrUnsupported("x"), true},
		{"raw error", errors.New("boom"), true},
		{"gorm not found di usecase tetap dicatat", gorm.ErrRecordNotFound, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRecordError(tc.err); got != tc.want {
				t.Fatalf("shouldRecordError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestTraceFunc_BusinessErrorNotRecorded(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrNotFound", stderr.ErrNotFound("data tidak ada")},
		{"ErrParam", stderr.ErrParam("param salah")},
		{"ErrInvRule", stderr.ErrInvRule("rule")},
		{"ErrInvHeader", stderr.ErrInvHeader("header")},
		{"ErrPermission", stderr.ErrPermission("akses")},
		{"ErrWaiting", stderr.ErrWaiting("masih proses")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exp.reset()
			logExp.reset()
			func() (err error) {
				_, done := TraceFunc(offCtx(), "Biz")
				defer done(&err)
				return tc.err
			}()

			spans := exp.reset()
			if len(spans) != 1 {
				t.Fatalf("expected 1 span, got %d", len(spans))
			}
			if len(spans[0].Events()) != 0 || spans[0].Status().Code != codes.Unset {
				t.Fatalf("business error must not be recorded: events=%v status=%+v", spans[0].Events(), spans[0].Status())
			}
			if recs := logExp.reset(); len(recs) != 0 {
				t.Fatalf("expected no error log, got %d", len(recs))
			}
		})
	}
}

func TestTraceFunc_SystemErrorRecorded(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrRuntime", stderr.ErrRuntime("db down")},
		{"ErrThirdParty", stderr.ErrThirdParty("core banking")},
		{"raw", errors.New("boom")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exp.reset()
			logExp.reset()
			func() (err error) {
				_, done := TraceFunc(offCtx(), "Sys")
				defer done(&err)
				return tc.err
			}()

			spans := exp.reset()
			if len(spans) != 1 {
				t.Fatalf("expected 1 span, got %d", len(spans))
			}
			if len(spans[0].Events()) != 1 || spans[0].Status().Code != codes.Error {
				t.Fatalf("system error must be recorded: events=%v status=%+v", spans[0].Events(), spans[0].Status())
			}
			if recs := logExp.reset(); len(recs) != 1 {
				t.Fatalf("expected 1 error log, got %d", len(recs))
			}
		})
	}
}

func TestTraceDB_RecordNotFoundNotRecorded(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"direct", gorm.ErrRecordNotFound},
		{"wrapped", fmt.Errorf("get rekanan: %w", gorm.ErrRecordNotFound)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exp.reset()
			func() (err error) {
				_, done := TraceDB(offCtx(), "mst_registrasi", "SELECT")
				defer done(&err)
				return tc.err
			}()

			spans := exp.reset()
			if len(spans) != 1 {
				t.Fatalf("expected 1 span, got %d", len(spans))
			}
			s := spans[0]
			if len(s.Events()) != 0 || s.Status().Code != codes.Unset {
				t.Fatalf("not found must not be an error: events=%v status=%+v", s.Events(), s.Status())
			}
			if strAttr(s, "db.result") != "not_found" {
				t.Fatalf("db.result attribute missing: %v", s.Attributes())
			}
		})
	}
}

func TestTraceDB_OtherErrorRecorded(t *testing.T) {
	for _, e := range []error{errors.New("connection refused"), gorm.ErrDuplicatedKey} {
		exp.reset()
		func() (err error) {
			_, done := TraceDB(offCtx(), "mst_registrasi", "INSERT")
			defer done(&err)
			return e
		}()

		spans := exp.reset()
		if len(spans) != 1 || len(spans[0].Events()) != 1 || spans[0].Status().Code != codes.Error {
			t.Fatalf("db error %v must be recorded: %+v", e, spans)
		}
		if strAttr(spans[0], "db.result") != "" {
			t.Fatalf("db.result must only be set for not_found")
		}
	}
}

func TestTraceDone_NilErrorPointers(t *testing.T) {
	exp.reset()
	logExp.reset()
	var nilErr error
	_, doneFunc := TraceFunc(offCtx(), "NilPtr")
	doneFunc(nil)
	_, doneFunc2 := TraceFunc(offCtx(), "NilErr")
	doneFunc2(&nilErr)
	_, doneDB := TraceDB(offCtx(), "t", "SELECT")
	doneDB(nil)
	_, doneDB2 := TraceDB(offCtx(), "t", "SELECT")
	doneDB2(&nilErr)

	spans := exp.reset()
	if len(spans) != 4 {
		t.Fatalf("every span must still end, got %d", len(spans))
	}
	for _, s := range spans {
		if len(s.Events()) != 0 || s.Status().Code != codes.Unset {
			t.Fatalf("nothing must be recorded: %+v", s.Status())
		}
	}
	if recs := logExp.reset(); len(recs) != 0 {
		t.Fatalf("expected no log, got %d", len(recs))
	}
}
