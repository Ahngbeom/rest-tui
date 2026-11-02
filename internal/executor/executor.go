// Package executor sends a parsed, variable-resolved httpfile.Request over
// the network and captures its response.
package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Ahngbeom/rest-tui/internal/httpfile"
)

// Response is the captured result of executing a request.
type Response struct {
	StatusCode int
	Status     string
	Headers    []httpfile.Header
	Body       []byte
	Duration   time.Duration
	// Redirects lists followed hops as "<status> → <METHOD> <URL>"; nil if none.
	Redirects []string
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

	var redirects []string
	client := &http.Client{
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			redirects = append(redirects, fmt.Sprintf("%d → %s %s", next.Response.StatusCode, next.Method, next.URL.String()))
			return nil
		},
	}
	start := time.Now()
	httpResp, err := client.Do(httpReq)
	duration := time.Since(start)
	if err != nil {
		return nil, keepCtxCause(ctx, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, keepCtxCause(ctx, err)
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
		Redirects:  redirects,
	}, nil
}

// keepCtxCause makes errors.Is(err, context.Canceled/DeadlineExceeded) hold
// when ctx ended, even if the transport wrapped or replaced the cause.
func keepCtxCause(ctx context.Context, err error) error {
	cause := ctx.Err()
	if cause == nil || errors.Is(err, cause) {
		return err
	}
	return fmt.Errorf("%w: %w", cause, err)
}
