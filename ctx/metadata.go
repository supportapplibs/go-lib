package ctx

import (
	"context"
	"os"
	"strings"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/facades"
)

// keyMetadata custom type that can prevent collision.
type keyMetadata struct{}

var contextKey keyMetadata

// Metadata holds any request-scoped shared data within brispot microservice.
type Metadata struct {
	Authorization   string
	UserAgent       string
	CacheControl    string
	ForwardedFor    string
	RequestFrom     string
	DeviceId        string
	App             string
	VersionApp      string
	ReqId           string
	ReqTags         string
	ReqUser         string
	ReqNama         string
	ReqKodeJabatan  string
	ReqNamaJabatan  string
	ReqKodeUker     string
	ReqNamaUker     string
	ReqJenisUker    string
	ReqKodeMainUker string
	ReqKodeRegion   string
	PathGateway     string
	ApiKey          string
	UrlPath         string // UrlPath may contain the url path information coming from http request
	SignaturePath   string // SignaturePath may contain the signature command from artisan command
}

// Get retrieve Metadata from given context with key from this pkg.
func Get(ctx context.Context) Metadata {
	if mt, ok := ctx.Value(contextKey).(Metadata); ok {
		return mt
	}
	return Metadata{}
}

// SetFromRequestHeader set any available metadata from given http context in
// the request header.
func SetFromRequestHeader(c http.Context) {
	mt := Metadata{
		Authorization:   c.Request().Header("Authorization"),
		UserAgent:       c.Request().Header("User-Agent"),
		CacheControl:    c.Request().Header("Cache-Control"),
		ApiKey:          c.Request().Header("X-Api-Key"),
		ForwardedFor:    c.Request().Header("X-Forwarded-For"),
		RequestFrom:     c.Request().Header("X-Request-From"),
		DeviceId:        c.Request().Header("X-Device-Id"),
		App:             c.Request().Header("X-App"),
		VersionApp:      c.Request().Header("X-Version-App"),
		ReqId:           c.Request().Header("X-Request-Id"),
		ReqTags:         c.Request().Header("X-Request-Tags"),
		ReqUser:         c.Request().Header("X-Request-User"),
		ReqNama:         c.Request().Header("X-Request-Nama"),
		ReqKodeJabatan:  c.Request().Header("X-Request-Kode-Jabatan"),
		ReqNamaJabatan:  c.Request().Header("X-Request-Nama-Jabatan"),
		ReqKodeUker:     c.Request().Header("X-Request-Kode-Uker"),
		ReqNamaUker:     c.Request().Header("X-Request-Nama-Uker"),
		ReqKodeMainUker: c.Request().Header("X-Request-Kode-MainUker"),
		ReqKodeRegion:   c.Request().Header("X-Request-Kode-Region"),
		ReqJenisUker:    c.Request().Header("X-Request-Jenis-Uker"),
		PathGateway:     c.Request().Header("X-Path-Gateway"),
		UrlPath:         c.Request().Path(),
	}
	c.WithValue(contextKey, mt)
}

func SetFromConsoleCtx(c console.Context) {
	mt := Metadata{
		ReqId:         GenerateTimeBasedID(),
		SignaturePath: strings.Join(c.Arguments(), " "),
		UserAgent:     func() string { x, _ := os.Hostname(); return x }(),
		App:           "console",
	}
	facades.App().Bind("supportapplibsCtx", func(app foundation.Application) (any, error) {
		return context.WithValue(context.Background(), contextKey, mt), nil
	})
}

// GetReqId extract request id from given context. This is a shortcut for Get
// with ReqId to get the request id from given context.
func GetReqId(c context.Context) string {
	return Get(c).ReqId
}
