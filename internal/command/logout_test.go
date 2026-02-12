// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/disco"
	"github.com/opentofu/svchost/svcauth"

	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/cliconfig/svcauthconfig"
)

func TestLogout(t *testing.T) {
	workDir := t.TempDir()
	credsSrc := cliconfig.EmptyCredentialsSourceForTests(filepath.Join(workDir, "credentials.tfrc.json"))

	t.Run("with no hostname", func(t *testing.T) {
		logoutView, logoutDone := testView(t)
		c := &LogoutCommand{
			Meta: Meta{
				WorkingDir: workdir.NewDir("."),
				View:       logoutView,
				Services: disco.New(
					disco.WithCredentials(credsSrc),
				),
			},
		}
		status := c.Run([]string{})
		output := logoutDone(t)
		if status != 1 {
			t.Fatalf("unexpected error code %d\nstderr:\n%s", status, output.Stderr())
		}

		if !strings.Contains(output.Stderr(), "The logout command expects exactly one argument") {
			t.Errorf("unexpected error message: %s", output.Stderr())
		}
	})

	testCases := map[string]struct {
		// Hostname to associate a pre-stored token
		hostname string
		// Command-line arguments
		args []string
		// true iff the token at hostname should be removed by the command
		shouldRemove bool
	}{
		"remove token for a hostname": {
			hostname:     "tfe.example.com",
			args:         []string{"tfe.example.com"},
			shouldRemove: true,
		},
		"logout does not remove tokens for the other hostnames": {
			hostname:     "tfe.example.com",
			args:         []string{"other-tfe.acme.com"},
			shouldRemove: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			host := svchost.Hostname(tc.hostname)
			token := svcauth.HostCredentialsToken("some-token")
			err := credsSrc.StoreForHost(t.Context(), host, token)
			if err != nil {
				t.Fatalf("unexpected error storing credentials: %s", err)
			}
			logoutView, logoutDone := testView(t)
			c := &LogoutCommand{
				Meta: Meta{
					WorkingDir: workdir.NewDir("."),
					View:       logoutView,
					Services: disco.New(
						disco.WithCredentials(credsSrc),
					),
				},
			}
			status := c.Run(tc.args)
			output := logoutDone(t)
			if status != 0 {
				t.Fatalf("unexpected error code %d\nstderr:\n%s", status, output.Stderr())
			}

			creds, err := credsSrc.ForHost(t.Context(), host)
			if err != nil {
				t.Errorf("failed to retrieve credentials: %s", err)
			}
			if tc.shouldRemove {
				if creds != nil {
					t.Errorf("wrong token %q; should have no token", svcauthconfig.HostCredentialsBearerToken(t, creds))
				}
			} else {
				if got, want := svcauthconfig.HostCredentialsBearerToken(t, creds), "some-token"; got != want {
					t.Errorf("wrong token %q; want %q", got, want)
				}
			}
		})
	}
}
