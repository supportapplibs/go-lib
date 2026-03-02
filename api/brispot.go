package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/supportapplibs/go-lib/ctx"
)

// NewHTTPClient return HTTPClient implementer that also set some metadata header
// before sending the request.
func NewHTTPClient() HTTPClient {
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

	return &httpClient{cl: &client}
}

type httpClient struct {
	cl *http.Client
}

func (h *httpClient) Call(req *http.Request, timeouts ...time.Duration) (HTTPResponse, error) {
	if !ctx.ShouldSkipContextHeaders(req.Context()) {
		ctx.SetHTTPRequestHeader(req)
	}
	reqTimeout := DEFAULT_TIMEOUT
	if len(timeouts) > 0 {
		reqTimeout = timeouts[0]
	}

	ctxWithTimeout, cancel := context.WithTimeout(req.Context(), reqTimeout)
	defer cancel()
	req = req.WithContext(ctxWithTimeout)

	var resp response
	res, err := h.cl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection error on HTTP request: %w", err)
	}
	defer res.Body.Close()

	resp.body, _ = io.ReadAll(res.Body)
	resp.statusCode = res.StatusCode
	resp.header = make(map[string][]string)
	resp.header = res.Header

	return &resp, nil
}
