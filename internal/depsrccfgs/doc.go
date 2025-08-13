// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package depsrccfgs deals with the "dependency source config" file format,
// which allows optionally including a file that has similar effects to running
// a custom registry or provider mirror but with all of the data specified
// statically in the filesystem, without running a separate network service.
//
// Files in this format are intended to be included in the same version control
// repository as the root module(s) they effect: either directly in the same
// directory as the root module, or in some parent directory that many different
// root modules share.
package depsrccfgs
