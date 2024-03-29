// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package moduledeps contains types that can be used to describe the
// providers required for all of the modules in a module tree.
//
// It does not itself contain the functionality for populating such
// data structures; that's in OpenTofu core, since this package intentionally
// does not depend on OpenTofu core to avoid package dependency cycles.
package moduledeps
