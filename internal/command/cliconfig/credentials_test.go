// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/svcauth"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/command/cliconfig/svcauthconfig"
)

func TestCredentialsForHost(t *testing.T) {
	credSrc := &CredentialsSource{
		configured: map[svchost.Hostname]cty.Value{
			"configured.example.com": cty.ObjectVal(map[string]cty.Value{
				"token": cty.StringVal("configured"),
			}),
			"unused.example.com": cty.ObjectVal(map[string]cty.Value{
				"token": cty.StringVal("incorrectly-configured"),
			}),
		},

		// We'll use a static source to stand in for what would normally be
		// a credentials helper program, since we're only testing the logic
		// for choosing when to delegate to the helper here. The logic for
		// interacting with a helper program is tested in the svcauth package.
		helper: readOnlyCredentialsStore{
			svcauth.StaticCredentialsSource(map[svchost.Hostname]svcauth.HostCredentials{
				"from-helper.example.com": svcauth.HostCredentialsToken("from-helper"),

				// This should be shadowed by the "configured" entry with the same
				// hostname above.
				"configured.example.com": svcauth.HostCredentialsToken("incorrectly-from-helper"),
			}),
		},
		helperType: "fake",
	}

	testReqAuthHeader := func(t *testing.T, creds svcauth.HostCredentials) string {
		t.Helper()

		if creds == nil {
			return ""
		}

		req, err := http.NewRequest("GET", "http://example.com/", nil)
		if err != nil {
			t.Fatalf("cannot construct HTTP request: %s", err)
		}
		creds.PrepareRequest(req)
		return req.Header.Get("Authorization")
	}

	t.Run("configured", func(t *testing.T) {
		creds, err := credSrc.ForHost(t.Context(), svchost.Hostname("configured.example.com"))
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if got, want := testReqAuthHeader(t, creds), "Bearer configured"; got != want {
			t.Errorf("wrong result\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("from helper", func(t *testing.T) {
		creds, err := credSrc.ForHost(t.Context(), svchost.Hostname("from-helper.example.com"))
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if got, want := testReqAuthHeader(t, creds), "Bearer from-helper"; got != want {
			t.Errorf("wrong result\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("not available", func(t *testing.T) {
		creds, err := credSrc.ForHost(t.Context(), svchost.Hostname("unavailable.example.com"))
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if got, want := testReqAuthHeader(t, creds), ""; got != want {
			t.Errorf("wrong result\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("set in environment", func(t *testing.T) {
		envName := "TF_TOKEN_configured_example_com"

		expectedToken := "configured-by-env"
		t.Setenv(envName, expectedToken)

		creds, err := credSrc.ForHost(t.Context(), svchost.Hostname("configured.example.com"))
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if creds == nil {
			t.Fatal("no credentials found")
		}

		if got := svcauthconfig.HostCredentialsBearerToken(t, creds); got != expectedToken {
			t.Errorf("wrong result\ngot: %s\nwant: %s", got, expectedToken)
		}
	})

	t.Run("punycode name set in environment", func(t *testing.T) {
		envName := "TF_TOKEN_env_xn--eckwd4c7cu47r2wf_com"

		expectedToken := "configured-by-env"
		t.Setenv(envName, expectedToken)

		hostname, _ := svchost.ForComparison("env.ドメイン名例.com")
		creds, err := credSrc.ForHost(t.Context(), hostname)

		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if creds == nil {
			t.Fatal("no credentials found")
		}

		if got := svcauthconfig.HostCredentialsBearerToken(t, creds); got != expectedToken {
			t.Errorf("wrong result\ngot: %s\nwant: %s", got, expectedToken)
		}
	})

	t.Run("hyphens can be encoded as double underscores", func(t *testing.T) {
		envName := "TF_TOKEN_env_xn____caf__dma_fr"
		expectedToken := "configured-by-fallback"

		t.Setenv(envName, expectedToken)

		hostname, _ := svchost.ForComparison("env.café.fr")
		creds, err := credSrc.ForHost(t.Context(), hostname)

		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if creds == nil {
			t.Fatal("no credentials found")
		}

		if got := svcauthconfig.HostCredentialsBearerToken(t, creds); got != expectedToken {
			t.Errorf("wrong result\ngot: %s\nwant: %s", got, expectedToken)
		}
	})

	t.Run("periods are ok", func(t *testing.T) {
		envName := "TF_TOKEN_configured.example.com"
		expectedToken := "configured-by-env"

		t.Setenv(envName, expectedToken)

		hostname, _ := svchost.ForComparison("configured.example.com")
		creds, err := credSrc.ForHost(t.Context(), hostname)

		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if creds == nil {
			t.Fatal("no credentials found")
		}

		if got := svcauthconfig.HostCredentialsBearerToken(t, creds); got != expectedToken {
			t.Errorf("wrong result\ngot: %s\nwant: %s", got, expectedToken)
		}
	})

	t.Run("casing is insensitive", func(t *testing.T) {
		envName := "TF_TOKEN_CONFIGUREDUPPERCASE_EXAMPLE_COM"
		expectedToken := "configured-by-env"

		t.Setenv(envName, expectedToken)

		hostname, _ := svchost.ForComparison("configureduppercase.example.com")
		creds, err := credSrc.ForHost(t.Context(), hostname)

		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if creds == nil {
			t.Fatal("no credentials found")
		}

		if got := svcauthconfig.HostCredentialsBearerToken(t, creds); got != expectedToken {
			t.Errorf("wrong result\ngot: %s\nwant: %s", got, expectedToken)
		}
	})
}

func TestCredentialsStoreForget(t *testing.T) {
	d := t.TempDir()

	mockCredsFilename := filepath.Join(d, "credentials.tfrc.json")

	cfg := &Config{
		// This simulates there being a credentials block manually configured
		// in some file _other than_ credentials.tfrc.json.
		Credentials: map[string]map[string]interface{}{
			"manually-configured.example.com": {
				"token": "manually-configured",
			},
		},
	}

	// We'll initially use a credentials source with no credentials helper at
	// all, and thus with credentials stored in the credentials file.
	credSrc := cfg.credentialsSource(
		"", nil,
		mockCredsFilename,
	)

	testReqAuthHeader := func(t *testing.T, creds svcauth.HostCredentials) string {
		t.Helper()

		if creds == nil {
			return ""
		}

		req, err := http.NewRequest("GET", "http://example.com/", nil)
		if err != nil {
			t.Fatalf("cannot construct HTTP request: %s", err)
		}
		creds.PrepareRequest(req)
		return req.Header.Get("Authorization")
	}

	// Because these store/forget calls have side-effects, we'll bail out with
	// t.Fatal (or equivalent) as soon as anything unexpected happens.
	// Otherwise downstream tests might fail in confusing ways.
	{
		err := credSrc.StoreForHost(
			t.Context(),
			svchost.Hostname("manually-configured.example.com"),
			svcauth.HostCredentialsToken("not-manually-configured"),
		)
		if err == nil {
			t.Fatalf("successfully stored for manually-configured; want error")
		}
		if _, ok := err.(ErrUnwritableHostCredentials); !ok {
			t.Fatalf("wrong error type %T; want ErrUnwritableHostCredentials", err)
		}
	}
	{
		err := credSrc.ForgetForHost(
			t.Context(),
			svchost.Hostname("manually-configured.example.com"),
		)
		if err == nil {
			t.Fatalf("successfully forgot for manually-configured; want error")
		}
		if _, ok := err.(ErrUnwritableHostCredentials); !ok {
			t.Fatalf("wrong error type %T; want ErrUnwritableHostCredentials", err)
		}
	}
	{
		// We don't have a credentials file at all yet, so this first call
		// must create it.
		err := credSrc.StoreForHost(
			t.Context(),
			svchost.Hostname("stored-locally.example.com"),
			svcauth.HostCredentialsToken("stored-locally"),
		)
		if err != nil {
			t.Fatalf("unexpected error storing locally: %s", err)
		}

		creds, err := credSrc.ForHost(t.Context(), svchost.Hostname("stored-locally.example.com"))
		if err != nil {
			t.Fatalf("failed to read back stored-locally credentials: %s", err)
		}

		if got, want := testReqAuthHeader(t, creds), "Bearer stored-locally"; got != want {
			t.Fatalf("wrong header value for stored-locally\ngot:  %s\nwant: %s", got, want)
		}

		got := readHostsInCredentialsFile(mockCredsFilename)
		want := map[svchost.Hostname]struct{}{
			svchost.Hostname("stored-locally.example.com"): struct{}{},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("wrong credentials file content\n%s", diff)
		}
	}

	// Now we'll switch to having a credential helper active.
	// If we were loading the real CLI config from disk here then this
	// entry would already be in cfg.Credentials, but we need to fake that
	// in the test because we're constructing this *Config value directly.
	cfg.Credentials["stored-locally.example.com"] = map[string]interface{}{
		"token": "stored-locally",
	}
	mockHelper := &mockCredentialsHelper{current: make(map[svchost.Hostname]cty.Value)}
	credSrc = cfg.credentialsSource(
		"mock", mockHelper,
		mockCredsFilename,
	)
	{
		err := credSrc.StoreForHost(
			t.Context(),
			svchost.Hostname("manually-configured.example.com"),
			svcauth.HostCredentialsToken("not-manually-configured"),
		)
		if err == nil {
			t.Fatalf("successfully stored for manually-configured with helper active; want error")
		}
	}
	{
		err := credSrc.StoreForHost(
			t.Context(),
			svchost.Hostname("stored-in-helper.example.com"),
			svcauth.HostCredentialsToken("stored-in-helper"),
		)
		if err != nil {
			t.Fatalf("unexpected error storing in helper: %s", err)
		}

		creds, err := credSrc.ForHost(t.Context(), svchost.Hostname("stored-in-helper.example.com"))
		if err != nil {
			t.Fatalf("failed to read back stored-in-helper credentials: %s", err)
		}

		if got, want := testReqAuthHeader(t, creds), "Bearer stored-in-helper"; got != want {
			t.Fatalf("wrong header value for stored-in-helper\ngot:  %s\nwant: %s", got, want)
		}

		// Nothing should have changed in the saved credentials file
		got := readHostsInCredentialsFile(mockCredsFilename)
		want := map[svchost.Hostname]struct{}{
			svchost.Hostname("stored-locally.example.com"): struct{}{},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("wrong credentials file content\n%s", diff)
		}
	}
	{
		// Because stored-locally is already in the credentials file, a new
		// store should be sent there rather than to the credentials helper.
		err := credSrc.StoreForHost(
			t.Context(),
			svchost.Hostname("stored-locally.example.com"),
			svcauth.HostCredentialsToken("stored-locally-again"),
		)
		if err != nil {
			t.Fatalf("unexpected error storing locally again: %s", err)
		}

		creds, err := credSrc.ForHost(t.Context(), svchost.Hostname("stored-locally.example.com"))
		if err != nil {
			t.Fatalf("failed to read back stored-locally credentials: %s", err)
		}

		if got, want := testReqAuthHeader(t, creds), "Bearer stored-locally-again"; got != want {
			t.Fatalf("wrong header value for stored-locally\ngot:  %s\nwant: %s", got, want)
		}
	}
	{
		// Forgetting a host already in the credentials file should remove it
		// from the credentials file, not from the helper.
		err := credSrc.ForgetForHost(
			t.Context(),
			svchost.Hostname("stored-locally.example.com"),
		)
		if err != nil {
			t.Fatalf("unexpected error forgetting locally: %s", err)
		}

		creds, err := credSrc.ForHost(t.Context(), svchost.Hostname("stored-locally.example.com"))
		if err != nil {
			t.Fatalf("failed to read back stored-locally credentials: %s", err)
		}

		if got, want := testReqAuthHeader(t, creds), ""; got != want {
			t.Fatalf("wrong header value for stored-locally\ngot:  %s\nwant: %s", got, want)
		}

		// Should not be present in the credentials file anymore
		got := readHostsInCredentialsFile(mockCredsFilename)
		want := map[svchost.Hostname]struct{}{}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("wrong credentials file content\n%s", diff)
		}
	}
	{
		err := credSrc.ForgetForHost(
			t.Context(),
			svchost.Hostname("stored-in-helper.example.com"),
		)
		if err != nil {
			t.Fatalf("unexpected error forgetting in helper: %s", err)
		}

		creds, err := credSrc.ForHost(t.Context(), svchost.Hostname("stored-in-helper.example.com"))
		if err != nil {
			t.Fatalf("failed to read back stored-in-helper credentials: %s", err)
		}

		if got, want := testReqAuthHeader(t, creds), ""; got != want {
			t.Fatalf("wrong header value for stored-in-helper\ngot:  %s\nwant: %s", got, want)
		}
	}

	{
		// Finally, the log in our mock helper should show that it was only
		// asked to deal with stored-in-helper, not stored-locally.
		got := mockHelper.log
		want := []mockCredentialsHelperChange{
			{
				Host:   svchost.Hostname("stored-in-helper.example.com"),
				Action: "store",
			},
			{
				Host:   svchost.Hostname("stored-in-helper.example.com"),
				Action: "forget",
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("unexpected credentials helper operation log\n%s", diff)
		}
	}
}

type mockCredentialsHelperChange struct {
	Host   svchost.Hostname
	Action string
}

type mockCredentialsHelper struct {
	current map[svchost.Hostname]cty.Value
	log     []mockCredentialsHelperChange
}

// Assertion that mockCredentialsHelper implements svcauth.CredentialsSource
var _ svcauth.CredentialsSource = (*mockCredentialsHelper)(nil)

func (s *mockCredentialsHelper) ForHost(_ context.Context, hostname svchost.Hostname) (svcauth.HostCredentials, error) {
	v, ok := s.current[hostname]
	if !ok {
		return nil, nil
	}
	return svcauthconfig.HostCredentialsFromObject(v), nil
}

func (s *mockCredentialsHelper) StoreForHost(_ context.Context, hostname svchost.Hostname, new svcauth.NewHostCredentials) error {
	s.log = append(s.log, mockCredentialsHelperChange{
		Host:   hostname,
		Action: "store",
	})
	s.current[hostname] = new.ToStore()
	return nil
}

func (s *mockCredentialsHelper) ForgetForHost(_ context.Context, hostname svchost.Hostname) error {
	s.log = append(s.log, mockCredentialsHelperChange{
		Host:   hostname,
		Action: "forget",
	})
	delete(s.current, hostname)
	return nil
}

// readOnlyCredentialsStore is an adapter to make a [svcauth.CredentialsSource]
// appear to implement [svcauth.CredentialsStore] for testing purposes.
//
// It only statically implements that larger set of methods. If any of the
// store-specific methods are called they will immediately return an error.
type readOnlyCredentialsStore struct {
	source svcauth.CredentialsSource
}

var _ svcauth.CredentialsStore = readOnlyCredentialsStore{}

// ForHost implements svcauth.CredentialsStore.
func (r readOnlyCredentialsStore) ForHost(ctx context.Context, host svchost.Hostname) (svcauth.HostCredentials, error) {
	return r.source.ForHost(ctx, host)
}

// ForgetForHost implements svcauth.CredentialsStore.
func (r readOnlyCredentialsStore) ForgetForHost(ctx context.Context, host svchost.Hostname) error {
	return fmt.Errorf("this credentials store is actually read-only, despite implementing svcauth.CredentialsSource")
}

// StoreForHost implements svcauth.CredentialsStore.
func (r readOnlyCredentialsStore) StoreForHost(ctx context.Context, host svchost.Hostname, credentials svcauth.NewHostCredentials) error {
	return fmt.Errorf("this credentials store is actually read-only, despite implementing svcauth.CredentialsSource")
}
