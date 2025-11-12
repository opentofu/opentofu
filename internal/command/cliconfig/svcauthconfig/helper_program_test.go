// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package svcauthconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/svcauth"
)

func TestHelperProgramCredentialsSource(t *testing.T) {
	// The helper script used in this test currently assumes a Unix-like
	// environment where scripts are directly executable based on their #!
	// line and where bash is available. This is an assumption we inherited
	// from our predecessor which we'd like to address someday, but for now
	// we'll just skip this test unless we're on Linux or macOS since those
	// are the two OSes most commonly used for OpenTofu development where
	// we can expect this to work. (Other unixes could potentially work
	// but we don't want to maintain a huge list here.)
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("this test only works on Unix-like systems")
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	program := filepath.Join(wd, "testdata", "helperprog", "test-helper")
	t.Logf("testing with helper at %s", program)

	src := NewHelperProgramCredentialsStore(program)

	t.Run("happy path", func(t *testing.T) {
		creds, err := src.ForHost(t.Context(), svchost.Hostname("example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := HostCredentialsBearerToken(t, creds), "example-token"; got != want {
			t.Errorf("wrong token %q; want %q", got, want)
		}
	})
	t.Run("no credentials", func(t *testing.T) {
		creds, err := src.ForHost(t.Context(), svchost.Hostname("nothing.example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if creds != nil {
			t.Errorf("got credentials; want nil")
		}
	})
	t.Run("unsupported credentials type", func(t *testing.T) {
		creds, err := src.ForHost(t.Context(), svchost.Hostname("other-cred-type.example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if creds != nil {
			t.Errorf("got credentials; want nil")
		}
	})
	t.Run("lookup error", func(t *testing.T) {
		_, err := src.ForHost(t.Context(), svchost.Hostname("fail.example.com"))
		if err == nil {
			t.Error("completed successfully; want error")
		}
	})
	t.Run("store happy path", func(t *testing.T) {
		err := src.StoreForHost(t.Context(), svchost.Hostname("example.com"), svcauth.HostCredentialsToken("example-token"))
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("store error", func(t *testing.T) {
		err := src.StoreForHost(t.Context(), svchost.Hostname("fail.example.com"), svcauth.HostCredentialsToken("example-token"))
		if err == nil {
			t.Error("completed successfully; want error")
		}
	})
	t.Run("forget happy path", func(t *testing.T) {
		err := src.ForgetForHost(t.Context(), svchost.Hostname("example.com"))
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("forget error", func(t *testing.T) {
		err := src.ForgetForHost(t.Context(), svchost.Hostname("fail.example.com"))
		if err == nil {
			t.Error("completed successfully; want error")
		}
	})
}
