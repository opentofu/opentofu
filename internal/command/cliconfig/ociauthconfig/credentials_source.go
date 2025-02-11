// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

import (
	"context"
	"fmt"
)

type CredentialsSource interface {
	CredentialsSpecificity() CredentialsSpecificity
	Credentials(ctx context.Context, env CredentialsLookupEnvironment) (Credentials, error)
	credentialsSourceImpl() // prevents implementations outside this package
}

func NewStaticCredentialsSource(creds Credentials, spec CredentialsSpecificity) CredentialsSource {
	return &staticCredentialsSource{
		creds: creds,
		spec:  spec,
	}
}

func NewDockerCredentialHelperCredentialsSource(helperName string, serverURL string, spec CredentialsSpecificity) CredentialsSource {
	return &dockerCredentialHelperCredentialSource{
		helperName: helperName,
		serverURL:  serverURL,
		spec:       spec,
	}
}

type staticCredentialsSource struct {
	creds Credentials
	spec  CredentialsSpecificity
}

var _ CredentialsSource = (*staticCredentialsSource)(nil)

func (s *staticCredentialsSource) CredentialsSpecificity() CredentialsSpecificity {
	return s.spec
}

func (s *staticCredentialsSource) Credentials(_ context.Context, _ CredentialsLookupEnvironment) (Credentials, error) {
	return s.creds, nil
}

func (s *staticCredentialsSource) credentialsSourceImpl() {}

type dockerCredentialHelperCredentialSource struct {
	helperName string
	serverURL  string
	spec       CredentialsSpecificity
}

var _ CredentialsSource = (*dockerCredentialHelperCredentialSource)(nil)

func (s *dockerCredentialHelperCredentialSource) CredentialsSpecificity() CredentialsSpecificity {
	return s.spec
}

func (s *dockerCredentialHelperCredentialSource) Credentials(ctx context.Context, env CredentialsLookupEnvironment) (Credentials, error) {
	result, err := env.QueryDockerCredentialHelper(ctx, s.helperName, s.serverURL)
	if err != nil {
		return Credentials{}, fmt.Errorf("from %q credential helper: %w", s.helperName, err)
	}
	return Credentials{
		username: result.Username,
		password: result.Secret,
		// Docker-style credential helpers cannot produce OAuth credentials
	}, nil
}

func (s *dockerCredentialHelperCredentialSource) credentialsSourceImpl() {}
