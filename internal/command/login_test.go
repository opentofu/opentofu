// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitchellh/cli"

	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/hashicorp/terraform-svchost/disco"

	"github.com/opentofu/opentofu/internal/command/cliconfig"
	oauthserver "github.com/opentofu/opentofu/internal/command/testdata/login-oauth-server"
	tfeserver "github.com/opentofu/opentofu/internal/command/testdata/login-tfe-server"
	"github.com/opentofu/opentofu/internal/command/webbrowser"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/version"
)

func TestLogin(t *testing.T) {
	// oauthserver.Handler is a stub OAuth server implementation that will,
	// on success, always issue a bearer token named "good-token".
	s := httptest.NewServer(oauthserver.Handler)
	defer s.Close()

	// tfeserver.Handler is a stub TFE API implementation which will respond
	// to ping and current account requests, when requests are authenticated
	// with token "good-token"
	ts := httptest.NewServer(tfeserver.Handler)
	defer ts.Close()

	loginTestCase := func(test func(t *testing.T, c *LoginCommand, ui *cli.MockUi), useBrowserLauncher bool) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			workDir := t.TempDir()

			// We'll use this context to avoid asynchronous tasks outliving
			// a single test run.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Do not use the NewMockUi initializer here, as we want to delay
			// the call to init until after setting up the input mocks
			ui := new(cli.MockUi)

			var browserLauncher webbrowser.Launcher = nil
			if useBrowserLauncher {
				browserLauncher = webbrowser.NewMockLauncher(ctx)
			}

			creds := cliconfig.EmptyCredentialsSourceForTests(filepath.Join(workDir, "credentials.tfrc.json"))
			svcs := disco.NewWithCredentialsSource(creds)
			svcs.SetUserAgent(httpclient.OpenTofuUserAgent(version.String()))

			svcs.ForceHostServices(svchost.Hostname("example.com"), map[string]interface{}{
				"login.v1": map[string]interface{}{
					// For this fake hostname we'll use a conventional OAuth flow,
					// with browser-based consent that we'll mock away using a
					// mock browser launcher below.
					"client": "anything-goes",
					"authz":  s.URL + "/authz",
					"token":  s.URL + "/token",
				},
			})
			svcs.ForceHostServices(svchost.Hostname("with-scopes.example.com"), map[string]interface{}{
				"login.v1": map[string]interface{}{
					// with scopes
					// mock browser launcher below.
					"client": "scopes_test",
					"authz":  s.URL + "/authz",
					"token":  s.URL + "/token",
					"scopes": []interface{}{"app1.full_access", "app2.read_only"},
				},
			})
			svcs.ForceHostServices(svchost.Hostname(tfeHost), map[string]interface{}{
				// This represents Terraform Cloud, which does not yet support the
				// login API, but does support its own bespoke tokens API.
				"tfe.v2":   ts.URL + "/api/v2",
				"tfe.v2.1": ts.URL + "/api/v2",
				"tfe.v2.2": ts.URL + "/api/v2",
				"motd.v1":  ts.URL + "/api/terraform/motd",
			})
			svcs.ForceHostServices(svchost.Hostname("tfe.acme.com"), map[string]interface{}{
				// This represents a Terraform Enterprise instance which does not
				// yet support the login API, but does support its own bespoke tokens API.
				"tfe.v2":   ts.URL + "/api/v2",
				"tfe.v2.1": ts.URL + "/api/v2",
				"tfe.v2.2": ts.URL + "/api/v2",
			})
			svcs.ForceHostServices(svchost.Hostname("unsupported.example.net"), map[string]interface{}{
				// This host intentionally left blank.
			})

			c := &LoginCommand{
				Meta: Meta{
					Ui:              ui,
					BrowserLauncher: browserLauncher,
					Services:        svcs,
				},
			}

			test(t, c, ui)
		}
	}

	t.Run("no hostname provided", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {
		status := c.Run([]string{})
		if status == 0 {
			t.Fatalf("successful exit; want error")
		}

		if got, want := ui.ErrorWriter.String(), "The login command expects exactly one argument"; !strings.Contains(got, want) {
			t.Fatalf("missing expected error message\nwant: %s\nfull output:\n%s", want, got)
		}
	}, true))

	t.Run(tfeHost+" (no login support)", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {
		// Enter "yes" at the consent prompt, then paste a token with some
		// accidental whitespace.
		defer testInputMap(t, map[string]string{
			"approve": "yes",
			"token":   "  good-token ",
		})()
		status := c.Run([]string{tfeHost})
		if status != 0 {
			t.Fatalf("unexpected error code %d\nstderr:\n%s", status, ui.ErrorWriter.String())
		}

		credsSrc := c.Services.CredentialsSource()
		creds, err := credsSrc.ForHost(svchost.Hostname(tfeHost))
		if err != nil {
			t.Errorf("failed to retrieve credentials: %s", err)
		}
		if got, want := creds.Token(), "good-token"; got != want {
			t.Errorf("wrong token %q; want %q", got, want)
		}
		if got, want := ui.OutputWriter.String(), "Welcome to the cloud backend!"; !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, but was:\n%s", want, got)
		}
	}, true))

	t.Run("example.com with authorization code flow", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {
		// Enter "yes" at the consent prompt.
		defer testInputMap(t, map[string]string{
			"approve": "yes",
		})()
		status := c.Run([]string{"example.com"})
		if status != 0 {
			t.Fatalf("unexpected error code %d\nstderr:\n%s", status, ui.ErrorWriter.String())
		}

		credsSrc := c.Services.CredentialsSource()
		creds, err := credsSrc.ForHost(svchost.Hostname("example.com"))
		if err != nil {
			t.Errorf("failed to retrieve credentials: %s", err)
		}
		if got, want := creds.Token(), "good-token"; got != want {
			t.Errorf("wrong token %q; want %q", got, want)
		}

		if got, want := ui.OutputWriter.String(), "OpenTofu has obtained and saved an API token."; !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, but was:\n%s", want, got)
		}
	}, true))

	t.Run("example.com results in no scopes", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {

		host, _ := c.Services.Discover("example.com")
		client, _ := host.ServiceOAuthClient("login.v1")
		if len(client.Scopes) != 0 {
			t.Errorf("unexpected scopes %q; expected none", client.Scopes)
		}
	}, true))

	t.Run("with-scopes.example.com with authorization code flow and scopes", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {
		// Enter "yes" at the consent prompt.
		defer testInputMap(t, map[string]string{
			"approve": "yes",
		})()
		status := c.Run([]string{"with-scopes.example.com"})
		if status != 0 {
			t.Fatalf("unexpected error code %d\nstderr:\n%s", status, ui.ErrorWriter.String())
		}

		credsSrc := c.Services.CredentialsSource()
		creds, err := credsSrc.ForHost(svchost.Hostname("with-scopes.example.com"))

		if err != nil {
			t.Errorf("failed to retrieve credentials: %s", err)
		}

		if got, want := creds.Token(), "good-token"; got != want {
			t.Errorf("wrong token %q; want %q", got, want)
		}

		if got, want := ui.OutputWriter.String(), "OpenTofu has obtained and saved an API token."; !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, but was:\n%s", want, got)
		}
	}, true))

	t.Run("with-scopes.example.com results in expected scopes", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {

		host, _ := c.Services.Discover("with-scopes.example.com")
		client, _ := host.ServiceOAuthClient("login.v1")

		expectedScopes := [2]string{"app1.full_access", "app2.read_only"}

		var foundScopes [2]string
		copy(foundScopes[:], client.Scopes)

		if foundScopes != expectedScopes || len(client.Scopes) != len(expectedScopes) {
			t.Errorf("unexpected scopes %q; want %q", client.Scopes, expectedScopes)
		}
	}, true))

	t.Run("TFE host without login support", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {
		// Enter "yes" at the consent prompt, then paste a token with some
		// accidental whitespace.
		defer testInputMap(t, map[string]string{
			"approve": "yes",
			"token":   "  good-token ",
		})()
		status := c.Run([]string{"tfe.acme.com"})
		if status != 0 {
			t.Fatalf("unexpected error code %d\nstderr:\n%s", status, ui.ErrorWriter.String())
		}

		credsSrc := c.Services.CredentialsSource()
		creds, err := credsSrc.ForHost(svchost.Hostname("tfe.acme.com"))
		if err != nil {
			t.Errorf("failed to retrieve credentials: %s", err)
		}
		if got, want := creds.Token(), "good-token"; got != want {
			t.Errorf("wrong token %q; want %q", got, want)
		}

		if got, want := ui.OutputWriter.String(), "Logged in to the cloud backend"; !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, but was:\n%s", want, got)
		}
	}, true))

	t.Run("TFE host without login support, incorrectly pasted token", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {
		// Enter "yes" at the consent prompt, then paste an invalid token.
		defer testInputMap(t, map[string]string{
			"approve": "yes",
			"token":   "good-tok",
		})()
		status := c.Run([]string{"tfe.acme.com"})
		if status != 1 {
			t.Fatalf("unexpected error code %d\nstderr:\n%s", status, ui.ErrorWriter.String())
		}

		credsSrc := c.Services.CredentialsSource()
		creds, err := credsSrc.ForHost(svchost.Hostname("tfe.acme.com"))
		if err != nil {
			t.Errorf("failed to retrieve credentials: %s", err)
		}
		if creds != nil {
			t.Errorf("wrong token %q; should have no token", creds.Token())
		}
	}, true))

	t.Run("host without login or TFE API support", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {
		status := c.Run([]string{"unsupported.example.net"})
		if status == 0 {
			t.Fatalf("successful exit; want error")
		}

		if got, want := ui.ErrorWriter.String(), "Error: Host does not support OpenTofu tokens API"; !strings.Contains(got, want) {
			t.Fatalf("missing expected error message\nwant: %s\nfull output:\n%s", want, got)
		}
	}, true))

	t.Run("answering no cancels", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {
		// Enter "no" at the consent prompt
		defer testInputMap(t, map[string]string{
			"approve": "no",
		})()
		status := c.Run([]string{tfeHost})
		if status != 1 {
			t.Fatalf("unexpected error code %d\nstderr:\n%s", status, ui.ErrorWriter.String())
		}

		if got, want := ui.ErrorWriter.String(), "Login cancelled"; !strings.Contains(got, want) {
			t.Fatalf("missing expected error message\nwant: %s\nfull output:\n%s", want, got)
		}
	}, true))

	t.Run("answering y cancels", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {
		// Enter "y" at the consent prompt
		defer testInputMap(t, map[string]string{
			"approve": "y",
		})()
		status := c.Run([]string{tfeHost})
		if status != 1 {
			t.Fatalf("unexpected error code %d\nstderr:\n%s", status, ui.ErrorWriter.String())
		}

		if got, want := ui.ErrorWriter.String(), "Login cancelled"; !strings.Contains(got, want) {
			t.Fatalf("missing expected error message\nwant: %s\nfull output:\n%s", want, got)
		}
	}, true))

	// The following test does not use browser MockLauncher() and forces `tofu login` command print URL
	// and wait for the callback with code.
	// There is no timeout in `tofu login` OAuth2 callback server code, so the only way to interrupt it
	// is to write to the shutdown channel (or complete the login process).
	t.Run("example.com Ctrl+C interrupts login command", loginTestCase(func(t *testing.T, c *LoginCommand, ui *cli.MockUi) {
		// Enter "yes" at the consent prompt.
		defer testInputMap(t, map[string]string{
			"approve": "yes",
		})()

		// override the command's shutdown channel so we can write to it
		abortCh := make(chan struct{})
		c.ShutdownCh = abortCh

		// statusCh will receive command Run result
		statusCh := make(chan int)
		go func() {
			statusCh <- c.Run([]string{"example.com"})
		}()

		// abort background Login command and wait for its result
		// removing the following lint results in default test timeout, since we don't run mocked webbrowser
		// and OAuth2 callback server will never get request with 'code'.
		abortCh <- struct{}{}
		status := <-statusCh
		if status != 1 {
			t.Fatalf("unexpected error code %d after interrupting the command\nstderr:\n%s", status, ui.ErrorWriter.String())
		}

		if got, want := ui.ErrorWriter.String(), "Action aborted"; !strings.Contains(got, want) {
			t.Fatalf("missing expected error message\nwant: %s\nfull output:\n%s", want, got)
		}
	}, false))
}
