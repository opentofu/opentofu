package oras

import (
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/backend"
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
