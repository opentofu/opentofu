// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package svcauthconfig

import (
	"net/http"
	"strings"
	"testing"

	"github.com/opentofu/svchost/svcauth"
)

// The symbols in this file are intended for use in tests only, and are
// not appropriate for use in normal code.

// HostCredentialsBearerToken is a testing helper implementing a little
// abstraction inversion to get the bearer token used by a credentials source
// even though the [svcauth.HostCredentials] API is designed to be generic over
// what kind of credentials it encloses.
//
// This only works for a [svcauth.HostCredentials] implementation whose
// behavior is to add an Authorization header field to the request using
// the "Bearer" scheme, in which case it returns whatever content appears
// after that scheme. HostCredentials implementations that don't match that
// pattern must be tested in a different way.
//
// This helper should not be used in non-test code. The svcauth API
// intentionally encapsulates the details of how credentials are applied to
// a request so that it can potentially be extended in future to support
// other authentication schemes such as HTTP Basic authentication.
func HostCredentialsBearerToken(t testing.TB, creds svcauth.HostCredentials) string {
	t.Helper()

	fakeReq, err := http.NewRequest("GET", "http://example.com/", nil)
	if err != nil {
		t.Fatalf("failed to create fake request: %s", err) // should be impossible, since we control all the inputs
	}
	creds.PrepareRequest(fakeReq)

	header := fakeReq.Header
	authz := header.Values("authorization")
	if len(authz) == 0 {
		t.Fatal("the svcauth.HostCredentials implementation did not add an Authorization header field")
	}
	if len(authz) > 1 {
		t.Fatalf("the svcauth.HostCredentials implementation added %d Authorization header fields; want exactly one", len(authz))
	}

	raw := strings.TrimPrefix(strings.ToLower(authz[0]), "bearer ")
	if len(raw) == len(authz[0]) {
		t.Fatal("the svchost.HostCredentials implemented added an Authorization header that does not use the Bearer scheme")
	}
	return strings.TrimSpace(raw)
}
