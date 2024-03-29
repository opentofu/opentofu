// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package blocktoattr includes some helper functions that can perform
// preprocessing on a HCL body where a configschema.Block schema is available
// in order to allow list and set attributes defined in the schema to be
// optionally written by the user as block syntax.
package blocktoattr
