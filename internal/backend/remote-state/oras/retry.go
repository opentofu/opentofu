package oras

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	orasErrcode "oras.land/oras-go/v2/registry/remote/errcode"
)

// RetryConfig defines the retry behavior for transient failures.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the initial one).
	// Default: 3
	MaxAttempts int

	// InitialBackoff is the initial backoff duration before the first retry.
	// Default: 1s
	InitialBackoff time.Duration

	// MaxBackoff is the maximum backoff duration between retries.
	// Default: 30s
	MaxBackoff time.Duration

	// BackoffMultiplier is the multiplier applied to backoff after each retry.
	// Default: 2.0
	BackoffMultiplier float64
}

// DefaultRetryConfig returns a sensible default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// withRetry executes the operation with retry logic for transient failures.
// It respects context cancellation and uses exponential backoff.
func withRetry[T any](ctx context.Context, cfg RetryConfig, operation func(ctx context.Context) (T, error)) (T, error) {
	var zero T
	var lastErr error

	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}

	backoff := cfg.InitialBackoff
	if backoff <= 0 {
		backoff = time.Second
	}

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		result, err := operation(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't retry if context is cancelled
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		// Don't retry non-transient errors
		if !isTransientError(err) {
			return zero, err
		}

		// Don't wait after the last attempt
		if attempt == cfg.MaxAttempts {
			break
		}

		// Wait before retry, respecting context cancellation
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(backoff):
		}

		// Calculate next backoff with exponential increase
		backoff = time.Duration(float64(backoff) * cfg.BackoffMultiplier)
		if cfg.MaxBackoff > 0 && backoff > cfg.MaxBackoff {
			backoff = cfg.MaxBackoff
		}
	}

	return zero, lastErr
}

// withRetryNoResult is a convenience wrapper for operations that don't return a value.
func withRetryNoResult(ctx context.Context, cfg RetryConfig, operation func(ctx context.Context) error) error {
	_, err := withRetry(ctx, cfg, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, operation(ctx)
	})
	return err
}

// isTransientError returns true if the error is likely transient and the operation
// should be retried.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Check for HTTP status codes that indicate transient failures
	var errResp *orasErrcode.ErrorResponse
	if errors.As(err, &errResp) {
		switch errResp.StatusCode {
		case http.StatusTooManyRequests,       // 429 - Rate limited
			http.StatusRequestTimeout,          // 408 - Request timeout
			http.StatusBadGateway,              // 502 - Bad gateway
			http.StatusServiceUnavailable,      // 503 - Service unavailable
			http.StatusGatewayTimeout:          // 504 - Gateway timeout
			return true
		}
		return false
	}

	// Check for network-level errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		// Retry on timeout or temporary network errors
		return netErr.Timeout() || netErr.Temporary()
	}

	// Fallback: best-effort check based on common error strings.
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection reset"):
		return true
	case strings.Contains(msg, "connection refused"):
		return true
	case strings.Contains(msg, "timeout"):
		return true
	case strings.Contains(msg, "temporary failure"):
		return true
	case strings.Contains(msg, "no such host"):
		return true
	case strings.Contains(msg, "eof"):
		return true
	default:
		return false
	}
}
