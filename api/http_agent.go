package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

// DEFAULT_TIMEOUT the default timeout of team specification.
var DEFAULT_TIMEOUT = 10 * time.Second

// response object that may be returned by Agent.
type response struct {
	statusCode int
	body       []byte
	header     map[string][]string
}

// GetStatusCode get the http status code.
func (r *response) GetStatusCode() int { return r.statusCode }

// GetBody get the raw response body in bytes.
func (r *response) GetBody() []byte { return r.body }

// GetHeaders get the copy the original response header, after combining the
// value using comma.
func (r *response) GetHeaders() map[string]string {
	var h = make(map[string]string)
	for k, v := range r.header {
		h[k] = strings.Join(v, ",")
	}
	return h
}

// GetHeader get the response header by the given key after combining the value
// using comma.
func (r *response) GetHeader(key string) string { return strings.Join(r.header[key], ",") }

// ToObject transform HTTPResponse.GetBody to any object using json encoding.
func ToObject[T any](obj HTTPResponse) (T, error) {
	var t T
	return t, sonic.ConfigFastest.Unmarshal(obj.GetBody(), &t)
}

type HTTPResponse interface {
	// GetStatusCode get the http status code.
	GetStatusCode() int
	// GetBody get the raw response body in bytes.
	GetBody() []byte
	// GetHeaders get the original response header, after combining the value to
	// one using comma.
	GetHeaders() map[string]string
	// GetHeader get the response header by the given key after combining the value
	// using comma.
	GetHeader(key string) string
}

type HTTPClient interface {
	// Call send given request using HTTP, and optionally set custom timeout if
	// provided, otherwise will use DEFAULT_TIMEOUT.
	//
	// This function also help setting any necessary metadata for supportapplibs using
	// ctx pkg that also come from this lib.
	Call(req *http.Request, timeouts ...time.Duration) (HTTPResponse, error)
}

type HTTPClientExternal interface {
	// Call send given request using HTTP, and optionally set custom timeout if
	// provided, otherwise will use DEFAULT_TIMEOUT.
	//
	// This function also help setting any necessary metadata for supportapplibs using
	// ctx pkg that also come from this lib.
	Call(requestClient context.Context, req *http.Request, timeouts ...time.Duration) (HTTPResponse, error)
}
