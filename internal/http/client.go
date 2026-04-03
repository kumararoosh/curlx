package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/arooshkumar/curlx/internal/auth"
)

// Request is a description of an HTTP call to make.
type Request struct {
	Method  string
	URL     string
	Body    string
	Headers map[string]string
}

// Response holds the result of an HTTP call.
type Response struct {
	StatusCode int
	Status     string
	Headers    http.Header
	Body       []byte
	Duration   time.Duration
}

// Do executes an HTTP request, injecting auth headers from the active context.
func Do(req Request, authCtx *auth.Context) (*Response, error) {
	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequest(req.Method, req.URL, body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	// Default headers.
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	httpReq.Header.Set("Accept", "application/json")

	// Merge caller-supplied headers.
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	// Inject auth.
	if authCtx != nil {
		switch authCtx.Type {
		case auth.TokenBearer:
			httpReq.Header.Set("Authorization", "Bearer "+authCtx.Token)
		case auth.TokenAPIKey:
			key := authCtx.HeaderKey
			if key == "" {
				key = "X-API-Key"
			}
			httpReq.Header.Set(key, authCtx.Token)
		case auth.TokenBasic:
			// Token expected as "user:password".
			parts := strings.SplitN(authCtx.Token, ":", 2)
			if len(parts) == 2 {
				httpReq.SetBasicAuth(parts[0], parts[1])
			}
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	start := time.Now()

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	elapsed := time.Since(start)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    resp.Header,
		Body:       respBody,
		Duration:   elapsed,
	}, nil
}
