package oras

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	orasErrcode "oras.land/oras-go/v2/registry/remote/errcode"
)

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "regular error",
			err:      errors.New("something went wrong"),
			expected: false,
		},
		{
			name: "429 Too Many Requests",
			err: &orasErrcode.ErrorResponse{
				StatusCode: http.StatusTooManyRequests,
			},
			expected: true,
		},
		{
			name: "502 Bad Gateway",
			err: &orasErrcode.ErrorResponse{
				StatusCode: http.StatusBadGateway,
			},
			expected: true,
		},
		{
			name: "503 Service Unavailable",
			err: &orasErrcode.ErrorResponse{
				StatusCode: http.StatusServiceUnavailable,
			},
			expected: true,
		},
		{
			name: "504 Gateway Timeout",
			err: &orasErrcode.ErrorResponse{
				StatusCode: http.StatusGatewayTimeout,
			},
			expected: true,
		},
		{
			name: "408 Request Timeout",
			err: &orasErrcode.ErrorResponse{
				StatusCode: http.StatusRequestTimeout,
			},
			expected: true,
		},
		{
			name: "404 Not Found (not transient)",
			err: &orasErrcode.ErrorResponse{
				StatusCode: http.StatusNotFound,
			},
			expected: false,
		},
		{
			name: "401 Unauthorized (not transient)",
			err: &orasErrcode.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
			},
			expected: false,
		},
		{
			name:     "error with connection reset in message",
			err:      errors.New("read tcp: connection reset by peer"),
			expected: true,
		},
		{
			name:     "error with connection refused in message",
			err:      errors.New("dial tcp: connection refused"),
			expected: true,
		},
		{
			name:     "error with timeout in message",
			err:      errors.New("connection timeout occurred"),
			expected: true,
		},
		{
			name:     "error with EOF in message",
			err:      errors.New("unexpected EOF"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTransientError(tt.err)
			if result != tt.expected {
				t.Errorf("isTransientError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestWithRetry_Success(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	attempts := 0
	result, err := withRetry(ctx, cfg, func(ctx context.Context) (string, error) {
		attempts++
		return "success", nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %q", result)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestWithRetry_TransientFailureThenSuccess(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	attempts := 0
	result, err := withRetry(ctx, cfg, func(ctx context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", &orasErrcode.ErrorResponse{StatusCode: http.StatusServiceUnavailable}
		}
		return "success", nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %q", result)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestWithRetry_NonTransientFailure(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	attempts := 0
	_, err := withRetry(ctx, cfg, func(ctx context.Context) (string, error) {
		attempts++
		return "", &orasErrcode.ErrorResponse{StatusCode: http.StatusUnauthorized}
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry for non-transient), got %d", attempts)
	}
}

func TestWithRetry_MaxAttemptsExhausted(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	attempts := 0
	_, err := withRetry(ctx, cfg, func(ctx context.Context) (string, error) {
		attempts++
		return "", &orasErrcode.ErrorResponse{StatusCode: http.StatusServiceUnavailable}
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestWithRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := RetryConfig{
		MaxAttempts:       5,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        1 * time.Second,
		BackoffMultiplier: 2.0,
	}

	attempts := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := withRetry(ctx, cfg, func(ctx context.Context) (string, error) {
		attempts++
		return "", &orasErrcode.ErrorResponse{StatusCode: http.StatusServiceUnavailable}
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestWithRetryNoResult(t *testing.T) {
	ctx := context.Background()
	cfg := RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}

	attempts := 0
	err := withRetryNoResult(ctx, cfg, func(ctx context.Context) error {
		attempts++
		if attempts < 2 {
			return &orasErrcode.ErrorResponse{StatusCode: http.StatusServiceUnavailable}
		}
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}
