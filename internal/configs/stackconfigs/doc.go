// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package configs contains types that represent OpenTofu stack configurations,
// which typically live in files with the ".stack.hcl" suffix.
//
// Stack configurations conceptually represent the call to a root module,
// dealing with various concerns that live "outside" of the root module such
// as the values for its input variables, and the state storage used for it.
//
// A stack configuration is selected when running "tofu init", and is then
// reused for subsequent OpenTofu workflow commands until a different
// stack is selected by running "tofu init" again.
package stackconfigs
