// Package executor sends a parsed, variable-resolved httpfile.Request over
// the network and captures its response.
package executor

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/bahn/rest-tui/internal/httpfile"
)

// Response is the captured result of executing a request.
type Response struct {
	StatusCode int
	Status     string
	Headers    []httpfile.Header
	Body       []byte
	Duration   time.Duration
}

// Execute sends req and waits up to timeout for a response. req.Method,
// req.URL, req.Headers, and req.Body must already have all {{variable}}
// placeholders resolved.
func Execute(ctx context.Context, req httpfile.Request, timeout time.Duration) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	if req.Body != "" {
		body = bytes.NewReader([]byte(req.Body))
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, body)
	if err != nil {
		return nil, err
	}
	for _, h := range req.Headers {
		httpReq.Header.Add(h.Name, h.Value)
	}

	client := &http.Client{}
	start := time.Now()
	httpResp, err := client.Do(httpReq)
	duration := time.Since(start)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}

	var headers []httpfile.Header
	for name, values := range httpResp.Header {
		for _, v := range values {
			headers = append(headers, httpfile.Header{Name: name, Value: v})
		}
	}

	return &Response{
		StatusCode: httpResp.StatusCode,
		Status:     httpResp.Status,
		Headers:    headers,
		Body:       respBody,
		Duration:   duration,
	}, nil
}
