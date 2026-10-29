package otellog

import (
	"errors"

	"github.com/redis/go-redis/v9"
	"github.com/supportapplibs/go-lib/stderr"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// ignoredBusinessErrorCodes adalah kode stderr yang merupakan hasil normal
// proses bisnis (validasi, data tidak ada, rule, header, permission, waiting), bukan
// kegagalan sistem. Error dengan kode ini tidak dicatat sebagai error di span
// maupun log, supaya error rate di APM tidak berisik. Kode 05 (masih menunggu
// proses background) juga termasuk hasil normal. Kode lain (04, 06, 99) dan
// error non-stderr tetap dicatat.
var ignoredBusinessErrorCodes = map[string]struct{}{
	stderr.ERROR_CODE_PARAMETER:         {},
	stderr.ERROR_CODE_DATA_NOT_FOUND:    {},
	stderr.ERROR_CODE_INVALID_RULE:      {},
	stderr.ERROR_CODE_INVALID_HEADER:    {},
	stderr.ERROR_CODE_ACCESS_PERMISSION: {},
	stderr.ERROR_CODE_WAITING_STATUS:    {},
}

// shouldRecordError menentukan apakah err dicatat sebagai error (RecordError,
// status Error, log Error) oleh TraceFunc/Trace. false untuk nil dan error
// bisnis stderr; true untuk error sistem dan error non-stderr.
func shouldRecordError(err error) bool {
	if err == nil {
		return false
	}
	if stderr.IsStdError(err) {
		_, ignored := ignoredBusinessErrorCodes[stderr.GetCode(err)]
		return !ignored
	}
	return true
}

// isNotFound true untuk gorm.ErrRecordNotFound dan redis.Nil (termasuk yang
// dibungkus %w): hasil normal lookup, bukan kegagalan sistem.
func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, redis.Nil)
}

// shouldRecordDBError sama dengan shouldRecordError untuk span DB/Redis,
// ditambah gorm.ErrRecordNotFound dan redis.Nil (termasuk yang dibungkus %w)
// yang dianggap hasil normal. Error lain (koneksi, syntax, constraint, timeout)
// tetap dicatat.
func shouldRecordDBError(err error) bool {
	if err == nil || isNotFound(err) {
		return false
	}
	return shouldRecordError(err)
}

// RecordSpanError mencatat err pada span yang dibuat manual (mis. span
// controller dari tracer.Start) dengan aturan yang sama seperti TraceFunc:
// error bisnis stderr dan gorm.ErrRecordNotFound dilewati, error sistem
// dicatat (RecordError + status Error). Pengganti span.RecordError(err):
//
//	if err != nil {
//	    otellog.RecordSpanError(span, err)
//	    return RespondError(ctx, err)
//	}
func RecordSpanError(span trace.Span, err error) {
	if shouldRecordDBError(err) {
		markSpanError(span, err)
	}
}
