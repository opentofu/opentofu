// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package hcl2shim contains a small number of "shimming" utilities that the
// other packages under internal/legacy use to adapt from HCL 2 concepts to
// legacy concepts.
//
// It's unfortunately also used in support of some very old tests in non-legacy
// packages that were originally written against the legacy packages, which are
// preserved in their current form for now to ensure that they keep testing what
// they were originally intended to test, but will hopefully be gradually
// modernized over time as they get updated for other reasons.
//
// Nothing in this package should be used in new code. It's here only to keep
// the other legacy code working.
package hcl2shim
