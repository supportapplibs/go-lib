package middleware

import (
	"mime/multipart"
	"slices"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/facades"
	"github.com/goravel/framework/filesystem"
	"github.com/supportapplibs/go-lib/ctx"
	"github.com/supportapplibs/go-lib/log"
	"github.com/supportapplibs/go-lib/stdresp"
)

const msgSizeLimit = 5000
const msgExceedLimit = "More than 5000 character"

// formDataFile holds information of each binary file in multipart form-data.
type formDataFile struct {
	Filename string `json:"filename"`
	Mimetype string `json:"mimetype"`
	Size     int    `json:"size"`
}

// ActivityMonitor capture and log all request/response.
func ActivityMonitor(c http.Context) {
	if c.Request().Path() == "/ping" || c.Request().Path() == "/favicon.ico" {
		return
	}
	now := time.Now()
	c.Request().Next()
	apiActivityRecorder(c, now)
}

func apiActivityRecorder(c http.Context, start time.Time) {
	ct := c.Request().Header("Content-Type")

	// check the content type and capture the request body according to it
	var req any
	switch {
	case hasPrefix(ct, "application/json", "application/x-www-form-urlencoded"):
		req = captureRequest(captureRequestMap(c))
	case hasPrefix(ct, "multipart/form-data"):
		req = captureRequest(captureRequestMultipart(c))
	default:
		req = captureRequest(captureRequestMap(c)) // treat any unhandled content-type as map
	}

	// get metadata from context
	mt := ctx.Get(c)

	activityData := map[string]any{
		"app_name":     facades.Config().GetString("app.name", "Microservice"),
		"host":         c.Request().Host(),
		"path":         c.Request().Path(),
		"clientip":     c.Request().Header("X-Forwarded-For", c.Request().Ip()),
		"clientapp":    mt.App,
		"path_alias":   mt.PathGateway,
		"requestID":    mt.ReqId,
		"requestFrom":  mt.RequestFrom,
		"requestUser":  mt.ReqUser,
		"deviceID":     mt.DeviceId,
		"requestTags":  mt.ReqTags,
		"requestBody":  req,
		"responseBody": captureResponse(c),
		"responseTime": time.Since(start).Milliseconds(),
		"httpCode":     c.Response().Origin().Status(),
		"requestAt":    start.Format(time.RFC3339Nano),
		"memoryUsage":  1024,
	}

	log.Activity(c).Info(activityData)
}

// captureRequest capture the request body if meet the criteria and requirement.
// Otherwise, will return msgExceedLimit.
func captureRequest(req map[string]any) any {
	// transform to json to make it easy to check the size
	b, _ := sonic.ConfigFastest.Marshal(req)
	if len(b) > msgSizeLimit {
		return msgExceedLimit
	}
	return req
}

// captureResponse capture the response body if meet the criteria and requirement.
func captureResponse(c http.Context) any {
	// transform back response to an object before capturing
	var res stdresp.Std
	if v := c.Response().Origin().Body(); v != nil {
		_ = sonic.ConfigFastest.Unmarshal(v.Bytes(), &res)

		// replace data if its len more than the limit 5000
		if len(v.Bytes()) > msgSizeLimit {
			res.ResponseData = msgExceedLimit
			return res
		}
	}
	return res
}

// captureRequestMap capture request as map and transform it to json string.
func captureRequestMap(c http.Context) map[string]any {
	return c.Request().All()
}

// captureRequestMultipart capture request multipart data and only get
// the information of each file such as the filename, size and extension.
// Include key-val form data but only pick the first value for each key.
func captureRequestMultipart(c http.Context) map[string]any {
	reqOrg := c.Request().Origin()
	_ = reqOrg.ParseMultipartForm(2 << 9) // 1024

	bagOfForm := make(map[string]any)
	// grab any available form-value
	for k, v := range reqOrg.MultipartForm.Value {
		if len(v) > 0 {
			bagOfForm[k] = v[0] // only pick the first data
		}
	}
	// grab any available files
	for field, header := range reqOrg.MultipartForm.File {
		var bagFormFiles []formDataFile
		for _, headerFile := range header {
			if headerFile != nil {
				bagFormFiles = append(bagFormFiles, formDataFile{
					Filename: headerFile.Filename,
					Size:     int(headerFile.Size),
					Mimetype: sniffMIMEType(headerFile),
				})
			}
		}
		bagOfForm[field] = bagFormFiles
	}

	return bagOfForm
}

// hasPrefix return true if the given s has at least one of the given prefixes.
func hasPrefix(s string, prefix ...string) bool {
	return slices.ContainsFunc(prefix, func(pre string) bool {
		return strings.HasPrefix(s, pre)
	})
}

// sniffMIMEType return the mime-type from given FileHeader instance by using
// helper provided by Goravel.
func sniffMIMEType(f *multipart.FileHeader) string {
	fl, err := filesystem.NewFileFromRequest(f)
	if err != nil {
		return "ERR-" + err.Error()
	}
	mt, err := fl.MimeType()
	if err != nil {
		return "ERR-" + err.Error()
	}
	return mt
}
