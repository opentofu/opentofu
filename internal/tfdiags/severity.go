// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfdiags

import (
	"fmt"
	"github.com/hashicorp/hcl/v2"
)

type SeverityLevel rune

//go:generate go run golang.org/x/tools/cmd/stringer -type=SeverityLevel -linecomment

const (
	ErrorLevel   SeverityLevel = 'E' // Error
	WarningLevel SeverityLevel = 'W' // Warning
)

type Severity struct {
	SeverityLevel
}

// ToHCL converts a Severity to the equivalent HCL diagnostic severity.
func (i Severity) ToHCL() hcl.DiagnosticSeverity {
	switch i.SeverityLevel {
	case WarningLevel:
		return hcl.DiagWarning
	case ErrorLevel:
		return hcl.DiagError
	default:
		// The above should always be exhaustive for all of the valid
		// Severity values in this package.
		panic(fmt.Sprintf("unknown diagnostic severity %s", i))
	}
}

var PedanticMode bool

// NewSeverity creates a new severity based on the level requested and whether we are running pedantic mode
func NewSeverity(level SeverityLevel) Severity {
	if PedanticMode && level == WarningLevel {
		return Severity{SeverityLevel: ErrorLevel}
	}
	return Severity{SeverityLevel: level}
}
