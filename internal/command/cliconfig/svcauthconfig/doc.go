// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package svcauthconfig contains some helper functions and types to support
// the cliconfig package's use of [github.com/opentofu/svchost/svcauth],
// which is our mechanism for representing the policy for authenticating to
// OpenTofu-native services such as implementations OpenTofu's provider registry
// protocol.
//
// The intended separation of concerns is that the upstream library provides
// the "vocabulary types" that other parts of OpenTofu interact with, while
// this package contains concrete implementations of those types and helpers
// to assist in constructing them which should be used _only_ by package
// cliconfig to satisfy the upstream interfaces. This separation means that
// we can evolve the implementation details of service authentication by
// changes only in this repository, thereby avoiding the complexity of always
// having to update both codebases in lockstep.
package svcauthconfig
