package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"time"

	"github.com/goravel/framework/facades"
	"github.com/redis/go-redis/v9"
	"github.com/supportapplibs/go-lib/ctx"
	"github.com/supportapplibs/go-lib/databases"
	"github.com/supportapplibs/go-lib/log"
)

// NewHTTPClientExternal return HTTPClient implementer that also set some metadata header
// before sending the request.
func NewHTTPClientExternal() HTTPClientExternal {
	var trans http.Transport
	trans.MaxConnsPerHost = 50
	trans.MaxIdleConnsPerHost = 15
	trans.MaxIdleConns = 50
	trans.IdleConnTimeout = 10 * time.Second
	trans.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	var client http.Client
	client.Transport = &trans
	client.Timeout = 30 * time.Second

	return &httpClientExternal{cl: &client}
}

type httpClientExternal struct {
	cl *http.Client
}

type MapRoute struct {
	Flag    bool   `json:"flag"`
	MockURL string `json:"mock_url"`
}

type ClientExternalSurroundingLog struct {
	AppName      string                 `json:"app_name"`
	Path         string                 `json:"path"`
	Host         string                 `json:"host"`
	Url          string                 `json:"url"`
	Request      SurroundingLogRequest  `json:"request"`
	Response     SurroundingLogResponse `json:"response"`
	ResponseTime time.Duration          `json:"response_time"`
	MemoryUsage  uint64                 `json:"memory_usage"`
}

type SurroundingLogRequest struct {
	Method string `json:"method"`
	Header any    `json:"header"`
	Body   any    `json:"body"`
}

type SurroundingLogResponse struct {
	HttpCode int `json:"http_code"`
	Header   any `json:"header"`
	Body     any `json:"body"`
}

func (h *httpClientExternal) Call(requestCtx context.Context, req *http.Request, timeouts ...time.Duration) (HTTPResponse, error) {
	// Init
	startTime := time.Now()
	metadata := ctx.Get(req.Context())

	// Read and store request body for logging
	var requestBody []byte
	var bodyDataLog interface{}
	if req.Body != nil {
		requestBody, _ = io.ReadAll(req.Body)
		req.Body.Close()

		// Parse as JSON, keep as object
		if json.Unmarshal(requestBody, &bodyDataLog) != nil {
			bodyDataLog = string(requestBody)
		}

		// Restore body for actual HTTP call
		req.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	}

	// Set Timeout
	reqTimeout := DEFAULT_TIMEOUT
	if len(timeouts) > 0 {
		reqTimeout = timeouts[0]
	}

	// Apply Mock
	reqUrl := req.URL.Scheme + "://" + req.URL.Host + req.URL.Path
	mapRoute, err := h.checkMock(reqUrl)
	if err == nil && mapRoute.Flag {
		mockURL, err := url.Parse(mapRoute.MockURL)
		if err == nil {
			req.URL = mockURL
			req.Host = mockURL.Host
		}
	}

	// Prepare Request
	ctxWithTimeout, cancel := context.WithTimeout(req.Context(), reqTimeout)
	defer cancel()
	req = req.WithContext(ctxWithTimeout)

	// Push Default Header
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "go-http-client/1.0")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "gzip, deflate")
	}

	// Call HTTP
	var resp response
	res, err := h.cl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection error on HTTP request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err = Body.Close()
		if err != nil {
			log.Runtime(ctxWithTimeout).Error(map[string]any{
				"msg": fmt.Sprintf("error closing response body: %v", err),
			})
		}
	}(res.Body)

	var bodyReader io.Reader = res.Body
	if res.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(res.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip decompress error: %w", err)
		}
		defer gzipReader.Close()
		bodyReader = gzipReader
	}

	// Responses
	resp.body, _ = io.ReadAll(bodyReader)
	resp.statusCode = res.StatusCode
	resp.header = make(map[string][]string)
	resp.header = res.Header

	// Calculate elapsed time and memory usage
	elapsed := time.Since(startTime)
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Parse response body as JSON object
	var bodyResponseLog interface{}
	if json.Unmarshal(resp.body, &bodyResponseLog) != nil {
		bodyResponseLog = string(resp.body)
	}

	// Populate ClientExternalSurroundingLog
	logData := ClientExternalSurroundingLog{
		AppName: facades.Config().GetString("APP_NAME"),
		Path:    getIdentifierPath(metadata),
		Host:    req.URL.Host,
		Url:     req.URL.Path,
		Request: SurroundingLogRequest{
			Method: req.Method,
			Header: req.Header,
			Body:   bodyDataLog,
		},
		Response: SurroundingLogResponse{
			HttpCode: res.StatusCode,
			Header:   res.Header,
			Body:     bodyResponseLog,
		},
		ResponseTime: elapsed,
		MemoryUsage:  m.Alloc,
	}

	const (
		msgValidate  = "more than 5000 characters"
		msgNotString = "not a string"
	)

	// Check Client Debug -> Response
	if facades.Config().GetString("CLIENT_DEBUG", "false") == "false" {
		if bodyStr, ok := logData.Response.Body.(string); ok {
			if len(bodyStr) > 5000 {
				logData.Response.Body = msgValidate
			}
		} else {
			// It's an object, check serialized length
			if serialized, err := json.Marshal(logData.Response.Body); err == nil {
				if len(serialized) > 5000 {
					logData.Response.Body = msgValidate
				}
			} else {
				logData.Response.Body = msgNotString
			}
		}
	}

	// Check Client Debug -> Request
	if facades.Config().GetString("CLIENT_DEBUG", "false") == "false" {
		if bodyStr, ok := logData.Request.Body.(string); ok {
			if len(bodyStr) > 5000 {
				logData.Request.Body = msgValidate
			}
		} else {
			// It's an object, check serialized length
			if serialized, err := json.Marshal(logData.Request.Body); err == nil {
				if len(serialized) > 5000 {
					logData.Request.Body = msgValidate
				}
			} else {
				logData.Request.Body = msgNotString
			}
		}
	}

	// Record Surrounding Log
	log.Activity(requestCtx).Info(h.externalCallLog(logData))

	// TODO -> Handle Log form-multipart

	return &resp, nil
}

func (h *httpClientExternal) checkMock(url string) (*MapRoute, error) {
	disallowedEnv := []string{"production", "staging", "piloting"}
	if slices.Contains(disallowedEnv, facades.Config().GetString("APP_ENV")) {
		return nil, errors.New("cannot use mock in production environment")
	}

	redisClient := databases.GetRedisClient()

	key := "eksternal_mock_url_mapping:" + url
	mapRouteData, err := redisClient.Get(context.Background(), key).Result()
	if errors.Is(err, redis.Nil) {
		return &MapRoute{}, nil
	}
	if err != nil {
		return nil, err
	}

	var mapRoute MapRoute
	err = json.Unmarshal([]byte(mapRouteData), &mapRoute)
	if err != nil {
		return nil, err
	}

	return &mapRoute, nil
}

func getIdentifierPath(metadata ctx.Metadata) string {
	if metadata.UrlPath != "" {
		return metadata.UrlPath
	}
	if metadata.SignaturePath != "" {
		return metadata.SignaturePath
	}
	return ""
}

func (h *httpClientExternal) externalCallLog(logData ClientExternalSurroundingLog) log.Map {
	return log.Map{
		"app_name":     logData.AppName,
		"path":         logData.Path,
		"host":         logData.Host,
		"url":          logData.Url,
		"request":      logData.Request,
		"response":     logData.Response,
		"responseTime": logData.ResponseTime.Milliseconds(),
		"memoryUsage":  logData.MemoryUsage,
	}
}
