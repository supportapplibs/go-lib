package middleware

import (
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/facades"
	"github.com/supportapplibs/go-lib/ctx"
)

// MetadataHeader set metadata information come from request header to current
// context.
func MetadataHeader(c http.Context) {
	if c.Request().Path() == "/ping" || c.Request().Path() == "/favicon.ico" {
		return
	}
	ctx.SetFromRequestHeader(c)
	facades.App().Bind("supportapplibsCtx", func(app foundation.Application) (any, error) { return c, nil })
	c.Request().Next()
}
