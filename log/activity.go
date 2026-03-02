package log

import (
	"context"
	"sync"

	"github.com/supportapplibs/go-lib/ctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	actOnce   sync.Once
	actZapLog *zap.Logger // actZapLog use zap log to log message, to benefit from its buffer.
)

type actLogger struct {
	trcId      string
	identifier string
}

func (l actLogger) Info(m Map) {
	m["traceID"] = l.trcId
	m["identifier"] = l.identifier
	actZapLog.Info("", zap.Any("payload", m))
}

// Activity start ActLogger.
func Activity(c context.Context) ActLogger {
	// prevent panic
	if c == nil {
		c = context.Background()
	}

	// use the no-op logger instead if the context contain off signal
	if v, ok := c.Value(logOffKey).(bool); ok && v {
		return noop{}
	}

	actOnce.Do(func() {
		// setup log writer
		actLogWriter := &writer{wr: setupLog("activity")}
		// setup zap log
		core := zapcore.NewCore(getZapJsonEncoder(), zapcore.AddSync(actLogWriter), zapcore.InfoLevel)
		actZapLog = zap.New(core)
		// add to clean up task
		cleanupTasks = append(cleanupTasks, func() {
			_ = actZapLog.Sync()
			_ = actLogWriter.Close()
		})
	})

	// - Start embedding any necessary metadata from context

	trcId := ctx.GetReqId(c)
	id := ctx.Get(c).UrlPath
	if id == "" {
		id = ctx.Get(c).SignaturePath
	}
	// add any other metadata here

	// - End embedding any necessary metadata from context

	return actLogger{trcId: trcId, identifier: id}
}
