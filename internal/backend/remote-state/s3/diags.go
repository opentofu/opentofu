// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"strings"

	"github.com/hashicorp/aws-sdk-go-base/v2/diag"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

func diagnosticString(d tfdiags.Diagnostic) string {
	var buffer strings.Builder
	buffer.WriteString(d.Severity().String() + ": ")
	buffer.WriteString(d.Description().Summary)
	if d.Description().Detail != "" {
		buffer.WriteString("\n\n")
		buffer.WriteString(d.Description().Detail)
	}
	return buffer.String()
}

func diagnosticsString(d tfdiags.Diagnostics) string {
	l := len(d)
	if l == 0 {
		return ""
	}

	var buffer strings.Builder
	for i, v := range d {
		buffer.WriteString(diagnosticString(v))
		if i < l-1 {
			buffer.WriteString(",\n")
		}
	}
	return buffer.String()
}

func baseSeverityToTofuSeverity(s diag.Severity) tfdiags.Severity {
	switch s {
	case diag.SeverityWarning:
		return tfdiags.NewSeverity(tfdiags.WarningLevel)
	case diag.SeverityError:
		return tfdiags.NewSeverity(tfdiags.ErrorLevel)
	default:
		return tfdiags.NewSeverity(-1)
	}
}
