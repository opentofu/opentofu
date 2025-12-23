package oras

import (
	"context"
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
