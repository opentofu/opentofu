// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package symlib is the implementation of the currently experimental symbols library
// proposal. It defines a hcl-base DSL for defining re-usable functions, types, and values
// to be used as libraries.
//
// Although currently included within OpenTofu, this package is intentionally decoupled
// from the rest of the codebase to facilitate being factored out into it's own library for
// use in other projects.

package symlib
