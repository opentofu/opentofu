// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package lint contains a collection of helpers for performing "lint-like"
// checks to try to detect configuration constructs that are valid but
// nonetheless very likely to be a mistake.
//
// A particular check only qualifies for inclusion here if its false-positive
// rate is very, very low. Incorrectly guessing that something was a mistake
// causes confusion.
package lint
