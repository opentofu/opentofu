// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graphviz

// EdgeAttachmentDirection models what Graphviz calls "compass points", which
// specify where on a node an edge should be attached.
//
// The constants of this type in this package are the only valid values of
// this type. Constructing other values of this type causes unspecified
// misbehavior.
type EdgeAttachmentDirection string

const (
	EdgeAttachmentAny       = EdgeAttachmentDirection("")
	EdgeAttachmentNorth     = EdgeAttachmentDirection(":n")
	EdgeAttachmentEast      = EdgeAttachmentDirection(":e")
	EdgeAttachmentSouth     = EdgeAttachmentDirection(":s")
	EdgeAttachmentWest      = EdgeAttachmentDirection(":w")
	EdgeAttachmentNorthEast = EdgeAttachmentDirection(":ne")
	EdgeAttachmentSouthEast = EdgeAttachmentDirection(":se")
	EdgeAttachmentNorthWest = EdgeAttachmentDirection(":nw")
	EdgeAttachmentSouthWest = EdgeAttachmentDirection(":sw")
	EdgeAttachmentCenter    = EdgeAttachmentDirection(":c")
)
