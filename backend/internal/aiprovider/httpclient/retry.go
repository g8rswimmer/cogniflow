// Package httpclient provides shared HTTP transport utilities for AI provider clients.
package httpclient

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"
)

// RetryTransport wraps an http.RoundTripper with automatic retry on transient
// failures (network errors, HTTP 429, HTTP 5xx) using exponential backoff with
// jitter. Request bodies must support GetBody; all requests constructed with
// bytes.NewReader or strings.NewReader qualify automatically via net/http.
type RetryTransport struct {
	base       http.RoundTripper
	maxRetries int
	baseDelay  time.Duration
}

// NewRetryTransport wraps base with retry logic. If base is nil,
// http.DefaultTransport is used. maxRetries is the number of additional
// attempts after the first (0 = no retry). baseDelay is the initial backoff
// duration; it doubles each attempt (capped at 30 s) with ±20 % jitter.
func NewRetryTransport(base http.RoundTripper, maxRetries int, baseDelay time.Duration) *RetryTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &RetryTransport{base: base, maxRetries: maxRetries, baseDelay: baseDelay}
}

// RoundTrip implements http.RoundTripper.
func (rt *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= rt.maxRetries; attempt++ {
		if attempt > 0 {
			// Rebuild request body so it can be re-read. GetBody is automatically
			// populated by net/http for bytes.NewReader and strings.NewReader bodies.
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("httpclient: rebuild request body for retry: %w", err)
				}
				req.Body = body
			}

			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(rt.backoff(attempt)):
			}
		}

		resp, err := rt.base.RoundTrip(req)
		if err != nil {
			// Do not retry if the request context was cancelled or timed out.
			if errors.Is(err, req.Context().Err()) && req.Context().Err() != nil {
				return nil, err
			}
			lastErr = err
			continue
		}

		if !isRetriableStatus(resp.StatusCode) {
			return resp, nil
		}

		// Drain and close the body before retrying so the connection is
		// returned to the pool rather than abandoned.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil, lastErr
}

// isRetriableStatus reports whether the status code indicates a transient
// server-side failure (rate limit or server error) that is safe to retry.
func isRetriableStatus(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code <= 599)
}

// backoff returns the delay before retry attempt n (1-based), doubling each
// attempt (capped at 30 s) with ±20 % jitter.
func (rt *RetryTransport) backoff(attempt int) time.Duration {
	d := rt.baseDelay
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > 30*time.Second {
			d = 30 * time.Second
			break
		}
	}
	// ±20 % jitter prevents thundering herd on simultaneous retries.
	spread := int64(d / 5)
	if spread > 0 {
		jitter := time.Duration(rand.Int64N(spread*2+1) - spread)
		d += jitter
	}
	if d < 0 {
		return 0
	}
	return d
}
