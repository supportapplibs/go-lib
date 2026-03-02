package stderr

import (
	"sync/atomic"

	"github.com/supportapplibs/go-lib/debug"
)

// debugDepthLevel hold the state the depth level of line captured in the debug.
var debugDepthLevel atomic.Int32

// errWithDebug internal helper to construct err with setting stack trace level
// to Debug Depth Level.
func errWithDebug(code string, msg string, httpCode int, metadata ...string) error {
	e := err{code: code, msg: msg, httpCode: httpCode, metadata: metadata}

	// switch debugDepthLevel.Load() {
	// case -1:
	//	e.stackTrc = debug.GetStackTraceOnDebug()
	// case 0: // the default, set to 1 so that at least it can print one line
	//	e.stackTrc = debug.GetStackTraceOnDebug(1)
	// default:
	//	e.stackTrc = debug.GetStackTraceOnDebug(int(debugDepthLevel.Load()))
	// }

	e.stackTrc = debug.GetStackTraceOnDebug()

	return e
}

// SetDebugDepthLevel set the depth level of line captured in the debug.
func SetDebugDepthLevel(lvl int) {
	debugDepthLevel.Store(int32(lvl))
}

// RemoveDebugDepthLevel remove the depth level so that error will print all
// captured stack trace.
func RemoveDebugDepthLevel() {
	debugDepthLevel.Store(int32(-1))
}
