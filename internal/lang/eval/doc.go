// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package eval aims to encapsulate the details of evaluating the objects
// in an overall configuration, including all of the expressions written inside
// their declarations, in a way that can be reused across various different
// phases of execution.
//
// The scope of this package intentionally excludes concepts like prior state,
// plans, etc, focusing only on dealing with the relationships between objects
// in a configuration. This package is therefore intended to be used as an
// implementation detail of higher-level operations like planning, with the
// caller using the provided hooks to incorporate the results of side-effects
// managed elsewhere.
package eval
