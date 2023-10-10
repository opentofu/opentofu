package s3

import (
	basediag "github.com/hashicorp/aws-sdk-go-base/v2/diag"
	"strings"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

func diagnosticString(diag tfdiags.Diagnostic) string {
	var buffer strings.Builder
	buffer.WriteString(diag.Severity().String() + ": ")
	buffer.WriteString(diag.Description().Summary)
	if diag.Description().Detail != "" {
		buffer.WriteString("\n\n")
		buffer.WriteString(diag.Description().Detail)
	}
	return buffer.String()
}

func diagnosticsString(diags tfdiags.Diagnostics) string {
	l := len(diags)
	if l == 0 {
		return ""
	}

	var buffer strings.Builder
	for i, d := range diags {
		buffer.WriteString(diagnosticString(d))
		if i < l-1 {
			buffer.WriteString(",\n")
		}
	}
	return buffer.String()
}

func baseSeverityToTofuSeverity(s basediag.Severity) tfdiags.Severity {
	switch s {
	case basediag.SeverityWarning:
		return tfdiags.Warning
	case basediag.SeverityError:
		return tfdiags.Error
	default:
		return -1
	}
}
