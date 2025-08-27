// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package grapheval contains some low-level helpers for coordinating
// interdependent work happening across different parts of the system, including
// detection and reporting of self-dependency problems that would otherwise
// cause a deadlock.
//
// The symbols in this package are intended to be implementation details of
// functionality in other packages, and not exposed as part of the public API
// of those packages.
package grapheval
