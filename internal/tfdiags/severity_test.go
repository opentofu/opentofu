package tfdiags

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewSeverity(t *testing.T) {
	testCases := []struct {
		name         string
		input        SeverityLevel
		expected     SeverityLevel
		withPedantic bool
	}{
		{"normal warning", WarningLevel, WarningLevel, false},
		{"pedantic warning", WarningLevel, ErrorLevel, true},
		{"normal error", ErrorLevel, ErrorLevel, false},
		{"pedantic error", ErrorLevel, ErrorLevel, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pedanticMode = tc.withPedantic
			assert.Equal(t, tc.expected, NewSeverity(tc.input).SeverityLevel)
			pedanticMode = false
		})
	}
}

func TestSeverityToHCL(t *testing.T) {
	testCases := []struct {
		name         string
		input        SeverityLevel
		expected     hcl.DiagnosticSeverity
		withPedantic bool
	}{
		{"normal hcl warning", WarningLevel, hcl.DiagWarning, false},
		{"pedantic hcl warning", WarningLevel, hcl.DiagError, true},
		{"normal hcl error", ErrorLevel, hcl.DiagError, false},
		{"pedantic hcl error", ErrorLevel, hcl.DiagError, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pedanticMode = tc.withPedantic
			assert.Equal(t, tc.expected, NewSeverity(tc.input).ToHCL())
			pedanticMode = false
		})
	}
}
