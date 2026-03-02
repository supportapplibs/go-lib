package middleware

import (
	"fmt"

	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/facades"
	"github.com/supportapplibs/go-lib/debug"
	"github.com/supportapplibs/go-lib/log"
	"github.com/supportapplibs/go-lib/stderr"
	"github.com/supportapplibs/go-lib/stdresp"
)

// Recover do recover when panic occurring anywhere in the stack. Also give
// back standardized response by utilizing stderr & stdresp pkg.
func Recover(c http.Context) {
	defer func() {
		// grab any panic occurring
		if r := recover(); r != nil {
			m := log.Map{"panic": "error panic recovered", "msg": debug.GetStackTraceInString(1)}
			log.Runtime(c).Error(m)

			prefixMsg := "Runtime Error - "
			suffixMsg := "Terjadi kesalahan, mohon coba beberapa saat lagi yaa..." // use masked message as the default
			if facades.Config().GetBool("APP_DEBUG") {
				// replace with the debug info if its enabled
				suffixMsg = fmt.Sprint(r) + " - " + m["msg"].(string)
			}

			err := stderr.Err(stderr.ERROR_CODE_SYSTEM, prefixMsg+suffixMsg, http.StatusOK)
			_ = stdresp.Error(c, stdresp.WithErr(err)).Render()
		}
	}()

	c.Request().Next()
}
