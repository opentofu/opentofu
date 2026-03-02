// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package resources contains helpers that encapsulate the main interactions
// OpenTofu has with resource instance objects, wrapping the raw provider client
// calls with certain preprocessing, postprocessing, and validation logic that
// ought to happen regardless of why OpenTofu is asking each of these questions.
package resources
