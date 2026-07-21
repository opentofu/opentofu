// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lsp

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func (s *server) publishDiagnostics(ctx context.Context, u uri.URI, src []byte) {
	filename, err := pathFromURI(u)

	if err != nil {
		filename = string(u)
	}

	_, diags := hclsyntax.ParseConfig(src, filename, hcl.InitialPos)
	_ = s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         u,
		Diagnostics: hclDiagsToLSP(diags)})

}

func hclDiagsToLSP(hclDiags hcl.Diagnostics) []protocol.Diagnostic {
	result := make([]protocol.Diagnostic, 0, len(hclDiags))

	for _, d := range hclDiags {
		sev := hclSevToLSP(d.Severity)
		rng := hclRangeToLSP(d)

		diag := protocol.Diagnostic{
			Severity: sev,
			Source:   protocol.NewOptional("tofu-ls"), // TODO: dont hard code
			Message:  protocol.String(fmt.Sprintf("%s: %s", d.Summary, d.Detail)),
		}

		if rng != nil {
			diag.Range = *rng
		}
		result = append(result, diag)

	}

	return result
}

func hclRangeToLSP(r *hcl.Diagnostic) *protocol.Range {
	if r == nil || r.Subject == nil {
		return nil
	}

	return &protocol.Range{
		Start: protocol.Position{Line: uint32(r.Subject.Start.Line - 1), Character: uint32(r.Subject.Start.Column - 1)},
		End:   protocol.Position{Line: uint32(r.Subject.End.Line - 1), Character: uint32(r.Subject.End.Column - 1)},
	}
}

func hclSevToLSP(sev hcl.DiagnosticSeverity) protocol.DiagnosticSeverity {
	switch sev {

	case hcl.DiagError:
		return protocol.DiagnosticSeverityError
	case hcl.DiagWarning:
		return protocol.DiagnosticSeverityWarning
	default:
		return protocol.DiagnosticSeverityHint // TODO: Decide here
	}
}
