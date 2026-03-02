package log

import (
	"context"
	"sync"

	"github.com/supportapplibs/go-lib/ctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	runOnce   sync.Once
	runZapLog *zap.Logger // runZapLog use zap log to log message, to benefit from its buffer.
)

type runLogger struct {
	trcId      string
	identifier string
}

func (l runLogger) Info(m Map) {
	m["traceID"] = l.trcId
	m["identifier"] = l.identifier
	runZapLog.Info("", zap.Any("payload", m))
}
func (l runLogger) Warning(m Map) {
	m["traceID"] = l.trcId
	m["identifier"] = l.identifier
	runZapLog.Warn("", zap.Any("payload", m))
}
func (l runLogger) Error(m Map) {
	m["traceID"] = l.trcId
	m["identifier"] = l.identifier
	runZapLog.Error("", zap.Any("payload", m))
}

// Runtime start RunLogger.
func Runtime(c context.Context) RunLogger {
	// prevent panic
	if c == nil {
		c = context.Background()
	}

	// use the no-op logger instead if the context contain off signal
	if v, ok := c.Value(logOffKey).(bool); ok && v {
		return noop{}
	}

	runOnce.Do(func() {
		// setup log writer
		runLogWriter := &writer{wr: setupLog("runtime")}
		// setup zap log
		core := zapcore.NewCore(getZapJsonEncoder(), zapcore.AddSync(runLogWriter), zapcore.InfoLevel)
		runZapLog = zap.New(core)
		// add to clean up task
		cleanupTasks = append(cleanupTasks, func() {
			_ = runZapLog.Sync()
			_ = runLogWriter.Close()
		})
	})

	// - Start embedding any necessary metadata from context

	trcId := ctx.GetReqId(c)
	id := ctx.Get(c).UrlPath
	// replace with signature in case coming from artisan command
	if id == "" {
		id = ctx.Get(c).SignaturePath
	}
	// add any other metadata here

	// - End embedding any necessary metadata from context

	return runLogger{trcId: trcId, identifier: id}
}
