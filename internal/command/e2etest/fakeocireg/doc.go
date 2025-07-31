// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package fakeocireg provides a minimal, read-only implementation of the OCI Distribution
// protocol that interacts with a local filesystem directory.
//
// This is intended only to support our end-to-end testing of installing dependencies from
// OCI Distribution repositories. It's not intended for use as a "real" OCI registry
// implementation.
package fakeocireg
