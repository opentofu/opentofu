package gitlab

import (
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/backend"
)

func TestBackendContract(_ *testing.T) {
	var _ backend.Backend = new(Backend)
}

func TestGitlabClientFactoryStatic(t *testing.T) {
	vars := map[string]cty.Value{
		"address":        cty.StringVal("http://127.0.0.1:8080"),
		"project":        cty.StringVal("namespace/project"),
		"token":          cty.StringVal("access-token"),
		"retry_max":      cty.StringVal("999"),
		"retry_wait_min": cty.StringVal("15"),
		"retry_wait_max": cty.StringVal("150"),
	}

	config := configs.SynthBody("synth", vars)

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), config).(*Backend)
	client := b.client

	if client == nil {
		t.Fatal("Unexpected failure: invalid config")
	}

	if client.BaseURL.String() != "http://127.0.0.1:8080" {
		t.Fatalf("Expected address \"%s\", got \"%s\"", vars["address"], client.BaseURL.String())
	}

	if client.Project != "namespace/project" {
		t.Fatalf("Expected project \"%s\", got \"%s\"", vars["project"], client.Project)
	}

	if client.StateName != backend.DefaultStateName {
		t.Fatalf("Expected state name \"%s\", got \"%s\"", client.StateName, backend.DefaultStateName)
	}

	if client.HTTPClient.RetryMax != 999 {
		t.Fatalf("Expected retry_max \"%d\", got \"%d\"", 999, client.HTTPClient.RetryMax)
	}

	if client.HTTPClient.RetryWaitMin != 15*time.Second {
		t.Fatalf("Expected retry_wait_min \"%s\", got \"%s\"", 15*time.Second, client.HTTPClient.RetryWaitMin)
	}

	if client.HTTPClient.RetryWaitMax != 150*time.Second {
		t.Fatalf("Expected retry_wait_max \"%s\", got \"%s\"", 150*time.Second, client.HTTPClient.RetryWaitMax)
	}
}

func TestGitlabClientFactoryFromEnv(t *testing.T) {
	conf := map[string]string{
		"address":        "http://127.0.0.1:8080",
		"project":        "namespace/project",
		"token":          "access-token",
		"retry_max":      "999",
		"retry_wait_min": "15",
		"retry_wait_max": "150",
	}

	t.Setenv("TF_GITLAB_ADDRESS", conf["address"])
	t.Setenv("TF_GITLAB_PROJECT", conf["project"])
	t.Setenv("TF_GITLAB_TOKEN", conf["token"])
	t.Setenv("TF_GITLAB_RETRY_MAX", conf["retry_max"])
	t.Setenv("TF_GITLAB_RETRY_WAIT_MIN", conf["retry_wait_min"])
	t.Setenv("TF_GITLAB_RETRY_WAIT_MAX", conf["retry_wait_max"])

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), nil).(*Backend)
	client := b.client

	if client == nil {
		t.Fatal("Unexpected failure: no config from env")
	}

	if client.BaseURL.String() != "http://127.0.0.1:8080" {
		t.Fatalf("Expected address \"%s\", got \"%s\"", conf["address"], client.BaseURL.String())
	}

	if client.Project != "namespace/project" {
		t.Fatalf("Expected project \"%s\", got \"%s\"", conf["project"], client.Project)
	}

	if client.StateName != backend.DefaultStateName {
		t.Fatalf("Expected state name \"%s\", got \"%s\"", client.StateName, backend.DefaultStateName)
	}

	if client.HTTPClient.RetryMax != 999 {
		t.Fatalf("Expected retry_max \"%d\", got \"%d\"", 999, client.HTTPClient.RetryMax)
	}

	if client.HTTPClient.RetryWaitMin != 15*time.Second {
		t.Fatalf("Expected retry_wait_min \"%s\", got \"%s\"", 15*time.Second, client.HTTPClient.RetryWaitMin)
	}

	if client.HTTPClient.RetryWaitMax != 150*time.Second {
		t.Fatalf("Expected retry_wait_max \"%s\", got \"%s\"", 150*time.Second, client.HTTPClient.RetryWaitMax)
	}
}
