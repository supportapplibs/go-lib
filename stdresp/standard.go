package stdresp

import (
	"github.com/goravel/framework/contracts/http"
	"github.com/supportapplibs/go-lib/ctx"
	"github.com/supportapplibs/go-lib/log"
	"github.com/supportapplibs/go-lib/stderr"
)

// Std holds standard response structure for brispot microservice.
type Std struct {
	httpCode           int      // httpCode hold http code that shall be controlled by stderr
	ResponseCode       string   `json:"responseCode"`
	ResponseDesc       string   `json:"responseDesc"`
	ResponseData       any      `json:"responseData,omitempty"`
	ResponseValidation []string `json:"responseValidation,omitempty"`
}

// Resp construct standard response after applying the response code and description
// along with any given options.
func Resp(c http.Context, code, desc string, opts ...StdOpt) http.Response {
	res := Std{
		httpCode:     http.StatusOK,
		ResponseCode: code,
		ResponseDesc: desc,
	}
	for _, opt := range opts {
		opt(&res)
	}

	// Auto Logging Error
	if res.ResponseCode == stderr.ERROR_CODE_SYSTEM {
		mt := ctx.Get(c) // Retrieve X-Request-Id
		log.Runtime(c).Error(log.Map{
			"code":      res.ResponseCode,
			"message":   res.ResponseDesc,
			"requestID": mt.ReqId,
		})
	}

	return c.Response().Json(res.httpCode, res)
}

// Success construct standard response with default success response code and
// description after applying any given options.
func Success(c http.Context, opts ...StdOpt) http.Response {
	return Resp(c, "00", "Success", opts...)
}

// Error construct standard response with default error response code and
// description after applying any given options.
func Error(c http.Context, opts ...StdOpt) http.Response {
	return Resp(c, stderr.ERROR_CODE_SYSTEM, stderr.ERROR_DESC_SYSTEM, opts...)
}
