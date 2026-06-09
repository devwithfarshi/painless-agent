package llm

import (
	"context"
	"errors"
	"math/rand/v2"
	"strings"
	"time"
)

// RetryProvider wraps any LLMProvider with exponential-backoff retry on transient errors.
type RetryProvider struct {
	inner      LLMProvider
	maxRetries int
}

// WithRetry wraps p with exponential-backoff retry (default 3 retries).
func WithRetry(p LLMProvider, maxRetries int) LLMProvider {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &RetryProvider{inner: p, maxRetries: maxRetries}
}

func (r *RetryProvider) Model() string { return r.inner.Model() }

func (r *RetryProvider) Complete(ctx context.Context, req CompletionRequest) (resp *Response, err error) {
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			if !retryWait(ctx, retryDelay(attempt)) {
				return nil, ctx.Err()
			}
		}
		resp, err = r.inner.Complete(ctx, req)
		if err == nil || !isTransient(err) {
			return
		}
	}
	return
}

func (r *RetryProvider) Stream(ctx context.Context, req CompletionRequest) (ch <-chan Delta, err error) {
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			if !retryWait(ctx, retryDelay(attempt)) {
				return nil, ctx.Err()
			}
		}
		ch, err = r.inner.Stream(ctx, req)
		if err == nil || !isTransient(err) {
			return
		}
	}
	return
}

func (r *RetryProvider) Embed(ctx context.Context, text string) (v []float32, err error) {
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			if !retryWait(ctx, retryDelay(attempt)) {
				return nil, ctx.Err()
			}
		}
		v, err = r.inner.Embed(ctx, text)
		if err == nil || !isTransient(err) {
			return
		}
	}
	return
}

// isTransient returns true for errors that are safe to retry.
// Provider errors include the HTTP status code in the message, e.g. "HTTP 429: ...".
func isTransient(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "HTTP 500") ||
		strings.Contains(msg, "HTTP 502") ||
		strings.Contains(msg, "HTTP 503") ||
		strings.Contains(msg, "HTTP 504") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "timeout")
}

// retryDelay returns exponential backoff with jitter for the given attempt (1-indexed).
func retryDelay(attempt int) time.Duration {
	base := time.Duration(1<<uint(attempt-1)) * time.Second
	if base > 30*time.Second {
		base = 30 * time.Second
	}
	jitter := time.Duration(rand.N(int64(base/2 + 1)))
	return base + jitter
}

func retryWait(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
