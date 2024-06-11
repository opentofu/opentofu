// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import "github.com/hashicorp/hcl/v2"

type Diagnostic interface {
	Severity() Severity
	Description() Description
	Source() Source

	// FromExpr returns the expression-related context for the diagnostic, if
	// available. Returns nil if the diagnostic is not related to an
	// expression evaluation.
	FromExpr() *FromExpr

	// ExtraInfo returns the raw extra information value. This is a low-level
	// API which requires some work on the part of the caller to properly
	// access associated information, so in most cases it'll be more convienient
	// to use the package-level ExtraInfo function to try to unpack a particular
	// specialized interface from this value.
	ExtraInfo() interface{}
}

type Description struct {
	Address string
	Summary string
	Detail  string
}

type Source struct {
	Subject *SourceRange
	Context *SourceRange
}

type FromExpr struct {
	Expression  hcl.Expression
	EvalContext *hcl.EvalContext
}
