// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package planning implements a planning engine for OpenTofu, which takes a
// prior state and a configuration instance (which can be evaluated to produce
// a desired state) and proposes a set of changes to make to bring the
// remote system closer to convergence with the desired state.
package planning
