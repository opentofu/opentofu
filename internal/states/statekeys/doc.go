// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package statekeys translates between our storage-layer representation of
// state keys and their semantic meaning within the current version of
// OpenTofu.
//
// These translations exist here to avoid them sprawling throughout the
// OpenTofu codebase, but THESE NAMING CONVENTIONS ARE NOT COVERED BY
// COMPATIBILITY PROMISES. The physical representation of state data in
// storage is private to OpenTofu and subject to change at any time.
package statekeys
