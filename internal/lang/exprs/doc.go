// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package exprs contains supporting code for expression evaluation.
//
// This package is designed to know nothing about the referenceable symbol tree
// in any particular language, so that knowledge can be kept closer to the
// other code implementing the relevant language. This can therefore be shared
// across many different HCL-based languages, and across different evaluation
// phases of the same language.
package exprs
