// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package oras

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/zclconf/go-cty/cty"
)

func TestBackend_impl(t *testing.T) {
	var _ backend.Backend = new(Backend)
}

func TestORASRetryConfigFromConfig(t *testing.T) {
	conf := map[string]cty.Value{
		"repository":     cty.StringVal("example.com/myorg/tofu-state"),
		"retry_max":      cty.StringVal("9"),
		"retry_wait_min": cty.StringVal("15"),
		"retry_wait_max": cty.StringVal("150"),
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), configs.SynthBody("synth", conf)).(*Backend)

	if b.retryCfg.MaxAttempts != 10 { // retry_max is number of retries
		t.Fatalf("expected MaxAttempts %d, got %d", 10, b.retryCfg.MaxAttempts)
	}
	if b.retryCfg.InitialBackoff != 15*time.Second {
		t.Fatalf("expected InitialBackoff %s, got %s", 15*time.Second, b.retryCfg.InitialBackoff)
	}
	if b.retryCfg.MaxBackoff != 150*time.Second {
		t.Fatalf("expected MaxBackoff %s, got %s", 150*time.Second, b.retryCfg.MaxBackoff)
	}
}

func TestORASRetryConfigFromEnv(t *testing.T) {
	t.Setenv(envVarRepository, "example.com/myorg/tofu-state")
	t.Setenv(envVarRetryMax, "9")
	t.Setenv(envVarRetryWaitMin, "15")
	t.Setenv(envVarRetryWaitMax, "150")

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), nil).(*Backend)

	if b.retryCfg.MaxAttempts != 10 {
		t.Fatalf("expected MaxAttempts %d, got %d", 10, b.retryCfg.MaxAttempts)
	}
	if b.retryCfg.InitialBackoff != 15*time.Second {
		t.Fatalf("expected InitialBackoff %s, got %s", 15*time.Second, b.retryCfg.InitialBackoff)
	}
	if b.retryCfg.MaxBackoff != 150*time.Second {
		t.Fatalf("expected MaxBackoff %s, got %s", 150*time.Second, b.retryCfg.MaxBackoff)
	}
}

func TestORASVersioningConfigFromConfig(t *testing.T) {
	conf := map[string]cty.Value{
		"repository":   cty.StringVal("example.com/myorg/tofu-state"),
		"max_versions": cty.StringVal("42"),
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), configs.SynthBody("synth", conf)).(*Backend)

	if b.versioningMaxVersions != 42 {
		t.Fatalf("expected versioningMaxVersions %d, got %d", 42, b.versioningMaxVersions)
	}
}

func TestORASCompressionConfigFromConfig(t *testing.T) {
	conf := map[string]cty.Value{
		"repository":  cty.StringVal("example.com/myorg/tofu-state"),
		"compression": cty.StringVal("gzip"),
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), configs.SynthBody("synth", conf)).(*Backend)
	if b.compression != "gzip" {
		t.Fatalf("expected compression %q, got %q", "gzip", b.compression)
	}
}

func TestORASLockTTLConfigFromConfig(t *testing.T) {
	conf := map[string]cty.Value{
		"repository": cty.StringVal("example.com/myorg/tofu-state"),
		"lock_ttl":   cty.StringVal("60"),
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), configs.SynthBody("synth", conf)).(*Backend)
	if b.lockTTL != 60*time.Second {
		t.Fatalf("expected lockTTL %s, got %s", 60*time.Second, b.lockTTL)
	}
}

func TestORASRateLimitConfigFromConfig(t *testing.T) {
	conf := map[string]cty.Value{
		"repository":       cty.StringVal("example.com/myorg/tofu-state"),
		"rate_limit":       cty.StringVal("10"),
		"rate_limit_burst": cty.StringVal("3"),
		"retry_max":        cty.StringVal("0"),
		"retry_wait_min":   cty.StringVal("1"),
		"retry_wait_max":   cty.StringVal("1"),
		"compression":      cty.StringVal("none"),
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), configs.SynthBody("synth", conf)).(*Backend)
	if b.rateLimit != 10 {
		t.Fatalf("expected rateLimit %d, got %d", 10, b.rateLimit)
	}
	if b.rateBurst != 3 {
		t.Fatalf("expected rateBurst %d, got %d", 3, b.rateBurst)
	}
}

type blockingLimiter struct {
	ch <-chan struct{}
}

func (l blockingLimiter) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.ch:
		return nil
	}
}

type countingRoundTripper struct {
	mu    sync.Mutex
	calls int
}

func (rt *countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.calls++
	rt.mu.Unlock()
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func (rt *countingRoundTripper) Calls() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.calls
}

func TestRateLimitedRoundTripper_WaitsBeforeRequest(t *testing.T) {
	gate := make(chan struct{})
	inner := &countingRoundTripper{}
	rt := &rateLimitedRoundTripper{limiter: blockingLimiter{ch: gate}, inner: inner}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_, _ = rt.RoundTrip(req)
		close(done)
	}()

	if inner.Calls() != 0 {
		t.Fatalf("expected no calls before limiter release")
	}

	close(gate)
	<-done

	if inner.Calls() != 1 {
		t.Fatalf("expected exactly 1 call after limiter release, got %d", inner.Calls())
	}
}

func TestUserAgentRoundTripper_DoesNotMutateOriginalRequest(t *testing.T) {
	inner := &countingRoundTripper{}
	rt := &userAgentRoundTripper{userAgent: "TestAgent/1.0", inner: inner}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	// Ensure the original request has no User-Agent set
	if req.Header.Get("User-Agent") != "" {
		t.Fatalf("expected no User-Agent on original request")
	}

	_, err = rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("roundtrip: %v", err)
	}

	// The original request must remain unmodified (RoundTripper contract)
	if got := req.Header.Get("User-Agent"); got != "" {
		t.Fatalf("original request was mutated: User-Agent = %q, want empty", got)
	}

	if inner.Calls() != 1 {
		t.Fatalf("expected 1 inner call, got %d", inner.Calls())
	}
}

func TestUserAgentRoundTripper_PreservesExistingUserAgent(t *testing.T) {
	var capturedUA string
	inner := &headerCapturingRoundTripper{capture: func(h http.Header) { capturedUA = h.Get("User-Agent") }}
	rt := &userAgentRoundTripper{userAgent: "TestAgent/1.0", inner: inner}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("User-Agent", "CustomAgent/2.0")

	_, err = rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("roundtrip: %v", err)
	}

	if capturedUA != "CustomAgent/2.0" {
		t.Fatalf("expected existing User-Agent to be preserved, got %q", capturedUA)
	}
}

type headerCapturingRoundTripper struct {
	capture func(http.Header)
}

func (rt *headerCapturingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.capture != nil {
		rt.capture(req.Header)
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

type countingLookupEnv struct {
	mu    sync.Mutex
	calls int

	result ociauthconfig.DockerCredentialHelperGetResult
	err    error
}

func (e *countingLookupEnv) QueryDockerCredentialHelper(ctx context.Context, helperName string, serverURL string) (ociauthconfig.DockerCredentialHelperGetResult, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	return e.result, e.err
}

func (e *countingLookupEnv) Calls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func TestCachedDockerCredentialHelperEnv_CachesSuccessWithinTTL(t *testing.T) {
	inner := &countingLookupEnv{
		result: ociauthconfig.DockerCredentialHelperGetResult{
			ServerURL: "https://example.com",
			Username:  "u",
			Secret:    "s",
		},
	}
	env := newCachedDockerCredentialHelperEnv(inner, time.Minute)
	base := time.Unix(100, 0)
	env.now = func() time.Time { return base }

	ctx := context.Background()
	_, err := env.QueryDockerCredentialHelper(ctx, "example", "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = env.QueryDockerCredentialHelper(ctx, "example", "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := inner.Calls(), 1; got != want {
		t.Fatalf("inner calls = %d, want %d", got, want)
	}
}

func TestCachedDockerCredentialHelperEnv_ExpiresAfterTTL(t *testing.T) {
	inner := &countingLookupEnv{
		result: ociauthconfig.DockerCredentialHelperGetResult{
			ServerURL: "https://example.com",
			Username:  "u",
			Secret:    "s",
		},
	}
	env := newCachedDockerCredentialHelperEnv(inner, 10*time.Second)
	base := time.Unix(100, 0)
	now := base
	env.now = func() time.Time { return now }

	ctx := context.Background()
	_, err := env.QueryDockerCredentialHelper(ctx, "example", "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now = base.Add(11 * time.Second)
	_, err = env.QueryDockerCredentialHelper(ctx, "example", "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := inner.Calls(), 2; got != want {
		t.Fatalf("inner calls = %d, want %d", got, want)
	}
}

func TestCachedDockerCredentialHelperEnv_CachesNotFoundErrorWithinTTL(t *testing.T) {
	notFoundErr := ociauthconfig.NewCredentialsNotFoundError(context.Canceled) // any wrapped error is fine
	inner := &countingLookupEnv{err: notFoundErr}
	env := newCachedDockerCredentialHelperEnv(inner, time.Minute)
	base := time.Unix(100, 0)
	env.now = func() time.Time { return base }

	ctx := context.Background()
	_, err := env.QueryDockerCredentialHelper(ctx, "example", "https://example.com")
	if err == nil || !ociauthconfig.IsCredentialsNotFoundError(err) {
		t.Fatalf("expected not-found error, got %v", err)
	}
	_, err = env.QueryDockerCredentialHelper(ctx, "example", "https://example.com")
	if err == nil || !ociauthconfig.IsCredentialsNotFoundError(err) {
		t.Fatalf("expected not-found error, got %v", err)
	}

	if got, want := inner.Calls(), 1; got != want {
		t.Fatalf("inner calls = %d, want %d", got, want)
	}
}
