// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package ociauthconfig contains types used for describing OCI authentication settings,
// and helpers for discovering such settings from container engine configuration
// files as described in https://github.com/containers/image/blob/main/docs/containers-auth.json.5.md .
//
// The rest of OpenTofu should make use of this package only indirectly through
// package cliconfig, which is responsible for making the policy decisions about
// which sources of authentication credentials we will use.
//
// This package is focused on policy rather than mechanism, and so it uses adapter
// interfaces like [ConfigDiscoveryEnvironment] and [CredentialsLookupEnvironment]
// to interact with mechanisms provided by the caller, using dependency-inversion
// style.
package ociauthconfig
