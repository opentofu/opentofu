// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonentities

import (
	"bufio"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcled"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/lang/marks"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// These severities map to the tfdiags.Severity values, plus an explicit
// unknown in case that enum grows without us noticing here.
const (
	DiagnosticSeverityUnknown = "unknown"
	DiagnosticSeverityError   = "error"
	DiagnosticSeverityWarning = "warning"
)

// Diagnostic represents any tfdiags.Diagnostic value. The simplest form has
// just a severity, single line summary, and optional detail. If there is more
// information about the source of the diagnostic, this is represented in the
// range field.

type Diagnostic struct {
	Severity string             `json:"severity"`
	Summary  string             `json:"summary"`
	Detail   string             `json:"detail"`
	Address  string             `json:"address,omitempty"`
	Range    *DiagnosticRange   `json:"range,omitempty"`
	Snippet  *DiagnosticSnippet `json:"snippet,omitempty"`
}

// Pos represents a position in the source code.
type Pos struct {
	// Line is a one-based count for the line in the indicated file.
	Line int `json:"line"`

	// Column is a one-based count of Unicode characters from the start of the line.
	Column int `json:"column"`

	// Byte is a zero-based offset into the indicated file.
	Byte int `json:"byte"`
}

// DiagnosticRange represents the filename and position of the diagnostic
// subject. This defines the range of the source to be highlighted in the
// output. Note that the snippet may include additional surrounding source code
// if the diagnostic has a context range.
//
// The Start position is inclusive, and the End position is exclusive. Exact
// positions are intended for highlighting for human interpretation only and
// are subject to change.
type DiagnosticRange struct {
	Filename string `json:"filename"`
	Start    Pos    `json:"start"`
	End      Pos    `json:"end"`
}

// DiagnosticSnippet represents source code information about the diagnostic.
// It is possible for a diagnostic to have a source (and therefore a range) but
// no source code can be found. In this case, the range field will be present and
// the snippet field will not.
type DiagnosticSnippet struct {
	// Context is derived from HCL's hcled.ContextString output. This gives a
	// high-level summary of the root context of the diagnostic: for example,
	// the resource block in which an expression causes an error.
	Context *string `json:"context"`

	// Code is a possibly-multi-line string of OpenTofu configuration, which
	// includes both the diagnostic source and any relevant context as defined
	// by the diagnostic.
	Code string `json:"code"`

	// StartLine is the line number in the source file for the first line of
	// the snippet code block. This is not necessarily the same as the value of
	// Range.Start.Line, as it is possible to have zero or more lines of
	// context source code before the diagnostic range starts.
	StartLine int `json:"start_line"`

	// HighlightStartOffset is the character offset into Code at which the
	// diagnostic source range starts, which ought to be highlighted as such by
	// the consumer of this data.
	HighlightStartOffset int `json:"highlight_start_offset"`

	// HighlightEndOffset is the character offset into Code at which the
	// diagnostic source range ends.
	HighlightEndOffset int `json:"highlight_end_offset"`

	// Values is a sorted slice of expression values which may be useful in
	// understanding the source of an error in a complex expression.
	Values []DiagnosticExpressionValue `json:"values"`

	// FunctionCall is information about a function call whose failure is
	// being reported by this diagnostic, if any.
	FunctionCall *DiagnosticFunctionCall `json:"function_call,omitempty"`
}

// DiagnosticExpressionValue represents an HCL traversal string (e.g.
// "var.foo") and a statement about its value while the expression was
// evaluated (e.g. "is a string", "will be known only after apply"). These are
// intended to help the consumer diagnose why an expression caused a diagnostic
// to be emitted.
type DiagnosticExpressionValue struct {
	Traversal string `json:"traversal"`
	Statement string `json:"statement"`
}

// DiagnosticFunctionCall represents a function call whose information is
// being included as part of a diagnostic snippet.
type DiagnosticFunctionCall struct {
	// CalledAs is the full name that was used to call this function,
	// potentially including namespace prefixes if the function does not belong
	// to the default function namespace.
	CalledAs string `json:"called_as"`

	// Signature is a description of the signature of the function that was
	// called, if any. Might be omitted if we're reporting that a call failed
	// because the given function name isn't known, for example.
	Signature *Function `json:"signature,omitempty"`
}

// NewDiagnostic takes a tfdiags.Diagnostic and a map of configuration sources,
// and returns a [Diagnostic] object as a "UI-flavored" representation of the
// diagnostic.
func NewDiagnostic(diag tfdiags.Diagnostic, sources map[string]*hcl.File) *Diagnostic {
	var sev string
	switch diag.Severity() {
	case tfdiags.Error:
		sev = DiagnosticSeverityError
	case tfdiags.Warning:
		sev = DiagnosticSeverityWarning
	default:
		sev = DiagnosticSeverityUnknown
	}

	sourceRefs := diag.Source()
	highlightRange, snippetRange := prepareDiagnosticRanges(sourceRefs.Subject, sourceRefs.Context)

	// If the diagnostic has source location information then we will try to construct a snippet
	// showing a relevant portion of the source code.
	snippet := newDiagnosticSnippet(snippetRange, highlightRange, sources)
	if snippet != nil {
		// We might be able to annotate the snippet with some dynamic-expression-related information,
		// if this is a suitably-enriched diagnostic. These are not strictly part of the "snippet",
		// but we return them all together because the human-readable UI presents this information
		// all together as one UI element.
		snippet.Values = newDiagnosticExpressionValues(diag)
		snippet.FunctionCall = newDiagnosticSnippetFunctionCall(diag)
	}

	desc := diag.Description()
	return &Diagnostic{
		Severity: sev,
		Summary:  desc.Summary,
		Detail:   desc.Detail,
		Address:  desc.Address,
		Range:    newDiagnosticRange(highlightRange),
		Snippet:  snippet,
	}
}

// prepareDiagnosticRanges takes the raw subject and context source ranges from a
// diagnostic message and returns the more UI-oriented "highlight" and "snippet"
// ranges.
//
// The "highlight" range describes the characters that are considered to be the
// direct cause of the problem, and which are typically presented as underlined
// when producing human-readable diagnostics in a terminal that can support that.
//
// The "snippet" range describes a potentially-larger range of characters that
// should all be included in the source code snippet included in the diagnostic
// message. The highlight range is guaranteed to be contained within the
// snippet range. Some of our diagnostic messages use this, for example, to
// ensure that the whole of an expression gets included in the snippet even if
// the problem is just one operand of the expression and the expression is wrapped
// over multiple lines.
func prepareDiagnosticRanges(subject, context *tfdiags.SourceRange) (highlight, snippet *tfdiags.SourceRange) {
	if subject == nil {
		// If we don't even have a "subject" then we have no ranges to report at all.
		return nil, nil
	}

	// We'll borrow HCL's range implementation here, because it has some
	// handy features to help us produce a nice source code snippet.
	highlightRange := subject.ToHCL()

	// Some diagnostic sources fail to set the end of the subject range.
	if highlightRange.End == (hcl.Pos{}) {
		highlightRange.End = highlightRange.Start
	}

	snippetRange := highlightRange
	if context != nil {
		snippetRange = context.ToHCL()
	}

	// Make sure the snippet includes the highlight. This should be true
	// for any reasonable diagnostic, but we'll make sure.
	snippetRange = hcl.RangeOver(snippetRange, highlightRange)

	// Empty ranges result in odd diagnostic output, so extend the end to
	// ensure there's at least one byte in the snippet or highlight.
	if highlightRange.Empty() {
		highlightRange.End.Byte++
		highlightRange.End.Column++
	}
	if snippetRange.Empty() {
		snippetRange.End.Byte++
		snippetRange.End.Column++
	}

	retHighlight := tfdiags.SourceRangeFromHCL(highlightRange)
	retSnippet := tfdiags.SourceRangeFromHCL(snippetRange)
	return &retHighlight, &retSnippet
}

func newDiagnosticRange(highlightRange *tfdiags.SourceRange) *DiagnosticRange {
	if highlightRange == nil {
		// No particular range to report, then.
		return nil
	}

	return &DiagnosticRange{
		Filename: highlightRange.Filename,
		Start: Pos{
			Line:   highlightRange.Start.Line,
			Column: highlightRange.Start.Column,
			Byte:   highlightRange.Start.Byte,
		},
		End: Pos{
			Line:   highlightRange.End.Line,
			Column: highlightRange.End.Column,
			Byte:   highlightRange.End.Byte,
		},
	}
}

func newDiagnosticSnippet(snippetRange, highlightRange *tfdiags.SourceRange, sources map[string]*hcl.File) *DiagnosticSnippet {
	if snippetRange == nil || highlightRange == nil {
		// There is no code that is relevant to show in a snippet for this diagnostic.
		return nil
	}
	file, ok := sources[snippetRange.Filename]
	if !ok {
		// If we don't have the source code for the file that the snippet is supposed
		// to come from then we can't produce a snippet. (This tends to happen when
		// we're rendering a diagnostic from an unusual location that isn't actually
		// a source file, like an expression entered into the "tofu console" prompt.)
		return nil
	}
	src := file.Bytes
	if src == nil {
		// A file without any source bytes? Weird, but perhaps constructed artificially
		// for testing or for other unusual reasons.
		return nil
	}

	// If we get this far then we're going to do our best to return at least a minimal
	// snippet, though the level of detail depends on what other information we have
	// available.
	ret := &DiagnosticSnippet{
		StartLine: snippetRange.Start.Line,

		// Ensure that the default Values struct is an empty array, as this
		// makes consuming the JSON structure easier in most languages.
		Values: []DiagnosticExpressionValue{},
	}

	// Some callers pass us *hcl.File objects they directly constructed rather than
	// using the HCL parser, in which case they lack the "navigation metadata"
	// that HCL's parsers would generate. We need that metadata to extract the
	// context string below, so we'll make a best effort to obtain that metadata.
	file = tryHCLFileWithNavMetadata(file, snippetRange.Filename)

	// Some diagnostics may have a useful top-level context to add to
	// the code snippet output. This function needs a file with nav metadata
	// to return a useful result, but it will happily return an empty string
	// if given a file without that metadata.
	contextStr := hcled.ContextString(file, highlightRange.Start.Byte-1)
	if contextStr != "" {
		ret.Context = &contextStr
	}

	// Build the string of the code snippet, tracking at which byte of
	// the file the snippet starts.
	var codeStartByte int
	sc := hcl.NewRangeScanner(src, highlightRange.Filename, bufio.ScanLines)
	var code strings.Builder
	for sc.Scan() {
		lineRange := sc.Range()
		if lineRange.Overlaps(snippetRange.ToHCL()) {
			if codeStartByte == 0 && code.Len() == 0 {
				codeStartByte = lineRange.Start.Byte
			}
			code.Write(lineRange.SliceBytes(src))
			code.WriteRune('\n')
		}
	}
	codeStr := strings.TrimSuffix(code.String(), "\n")
	ret.Code = codeStr

	// Calculate the start and end byte of the highlight range relative
	// to the code snippet string.
	start := highlightRange.Start.Byte - codeStartByte
	end := start + (highlightRange.End.Byte - highlightRange.Start.Byte)

	// We can end up with some quirky results here in edge cases like
	// when a source range starts or ends at a newline character,
	// so we'll cap the results at the bounds of the highlight range
	// so that consumers of this data don't need to contend with
	// out-of-bounds errors themselves.
	if start < 0 {
		start = 0
	} else if start > len(codeStr) {
		start = len(codeStr)
	}
	if end < 0 {
		end = 0
	} else if end > len(codeStr) {
		end = len(codeStr)
	}

	ret.HighlightStartOffset = start
	ret.HighlightEndOffset = end

	return ret
}

func newDiagnosticExpressionValues(diag tfdiags.Diagnostic) []DiagnosticExpressionValue {
	fromExpr := diag.FromExpr()
	if fromExpr == nil {
		// no expression-related information on this diagnostic, but our
		// callers always want a non-nil slice in this case because that's
		// friendlier for JSON serialization.
		return make([]DiagnosticExpressionValue, 0)
	}

	// We may also be able to generate information about the dynamic
	// values of relevant variables at the point of evaluation, then.
	// This is particularly useful for expressions that get evaluated
	// multiple times with different values, such as blocks using
	// "count" and "for_each", or within "for" expressions.
	expr := fromExpr.Expression
	ctx := fromExpr.EvalContext
	vars := expr.Variables()
	values := make([]DiagnosticExpressionValue, 0, len(vars))
	seen := make(map[string]struct{}, len(vars))
	includeUnknown := tfdiags.DiagnosticCausedByUnknown(diag)
	includeSensitive := tfdiags.DiagnosticCausedBySensitive(diag)

Traversals:
	for _, traversal := range vars {
		// We want to describe as specific a value as possible but since
		// evaluation failed it's possible that the full traversal is
		// not actually valid, so we'll try gradually-shorter prefixes
		// of the traversal until we're able to find an associated
		// value, reporting the value of the entire top-level symbol
		// as our worst-case successful outcome.
		for len(traversal) >= 1 {
			val, diags := traversal.TraverseAbs(ctx)
			if diags.HasErrors() {
				// Skip anything that generates errors, since we probably
				// already have the same error in our diagnostics set
				// already.
				traversal = traversal[:len(traversal)-1]
				continue
			}

			traversalStr := traversalStr(traversal)
			if _, exists := seen[traversalStr]; exists {
				continue Traversals // don't show duplicates when the same variable is referenced multiple times
			}
			statement := newDiagnosticSnippetValueDescription(val, includeUnknown, includeSensitive)
			if statement == "" {
				// If we don't have anything to say about this value then we won't include
				// an entry for it at all.
				continue Traversals
			}
			values = append(values, DiagnosticExpressionValue{
				Traversal: traversalStr,
				Statement: statement,
			})
			seen[traversalStr] = struct{}{}
			continue Traversals
		}
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].Traversal < values[j].Traversal
	})
	return values
}

func newDiagnosticSnippetFunctionCall(diag tfdiags.Diagnostic) *DiagnosticFunctionCall {
	fromExpr := diag.FromExpr()
	if fromExpr == nil {
		return nil // no expression-related information on this diagnostic
	}
	callInfo := tfdiags.ExtraInfo[hclsyntax.FunctionCallDiagExtra](diag)
	if callInfo == nil || callInfo.CalledFunctionName() == "" {
		return nil // no function call information
	}

	ctx := fromExpr.EvalContext
	calledAs := callInfo.CalledFunctionName()
	baseName := calledAs
	if idx := strings.LastIndex(baseName, "::"); idx >= 0 {
		baseName = baseName[idx+2:]
	}
	ret := &DiagnosticFunctionCall{
		CalledAs: calledAs,
	}
	if f, ok := ctx.Functions[calledAs]; ok {
		ret.Signature = DescribeFunction(baseName, f)
	}
	return ret
}

func newDiagnosticSnippetValueDescription(val cty.Value, includeUnknown, includeSensitive bool) string {
	switch {
	case val.HasMark(marks.Sensitive):
		// We only mention a sensitive value if the diagnostic
		// we're rendering is explicitly marked as being
		// caused by sensitive values, because otherwise
		// readers tend to be misled into thinking the error
		// is caused by the sensitive value even when it isn't.
		if !includeSensitive {
			return ""
		}
		// Even when we do mention one, we keep it vague
		// in order to minimize the chance of giving away
		// whatever was sensitive about it.
		return "has a sensitive value"
	case !val.IsKnown():
		ty := val.Type()
		// We'll avoid saying anything about unknown or
		// "known after apply" unless the diagnostic is
		// explicitly marked as being caused by unknown
		// values, because otherwise readers tend to be
		// misled into thinking the error is caused by the
		// unknown value even when it isn't.
		if !includeUnknown {
			if ty == cty.DynamicPseudoType {
				return "" // if we can't even name the type then we'll say nothing at all
			}
			// We can at least say what the type is, without mentioning "known after apply" at all
			return fmt.Sprintf("is a %s", ty.FriendlyName())
		}
		switch {
		case ty == cty.DynamicPseudoType:
			return "will be known only after apply" // we don't even know what the type will be
		case ty.IsCollectionType():
			// If the unknown value has collection length refinements then we might at least
			// be able to give some hints about the expected length.
			valRng := val.Range()
			minLen := valRng.LengthLowerBound()
			maxLen := valRng.LengthUpperBound()
			const maxLimit = 1024 // (upper limit is just an arbitrary value to avoid showing distracting large numbers in the UI)
			switch {
			case minLen == maxLen:
				return fmt.Sprintf("is a %s of length %d, known only after apply", ty.FriendlyName(), minLen)
			case minLen != 0 && maxLen <= maxLimit:
				return fmt.Sprintf("is a %s with between %d and %d elements, known only after apply", ty.FriendlyName(), minLen, maxLen)
			case minLen != 0:
				return fmt.Sprintf("is a %s with at least %d elements, known only after apply", ty.FriendlyName(), minLen)
			case maxLen <= maxLimit:
				return fmt.Sprintf("is a %s with up to %d elements, known only after apply", ty.FriendlyName(), maxLen)
			default:
				return fmt.Sprintf("is a %s, known only after apply", ty.FriendlyName())
			}
		default:
			return fmt.Sprintf("is a %s, known only after apply", ty.FriendlyName())
		}
	default:
		return fmt.Sprintf("is %s", compactValueStr(val))
	}
}

// compactValueStr produces a compact, single-line summary of a given value
// that is suitable for display in the UI.
//
// For primitives it returns a full representation, while for more complex
// types it instead summarizes the type, size, etc to produce something
// that is hopefully still somewhat useful but not as verbose as a rendering
// of the entire data structure.
func compactValueStr(val cty.Value) string {
	// This is a specialized subset of value rendering tailored to producing
	// helpful but concise messages in diagnostics. It is not comprehensive
	// nor intended to be used for other purposes.

	if val.HasMark(marks.Sensitive) {
		// We check this in here just to make sure, but note that the caller
		// of compactValueStr ought to have already checked this and skipped
		// calling into compactValueStr anyway, so this shouldn't actually
		// be reachable.
		return "(sensitive value)"
	}

	// val could have deprecated marks as well, so we want to
	// unmark it first to eliminate the risk of panics.
	val = marks.RemoveDeepDeprecated(val)

	// WARNING: We've only checked that the value isn't sensitive _shallowly_
	// here, and so we must never show any element values from complex types
	// in here. However, it's fine to show map keys and attribute names because
	// those are never sensitive in isolation: the entire value would be
	// sensitive in that case.

	ty := val.Type()
	switch {
	case val.IsNull():
		return "null"
	case !val.IsKnown():
		// Should never happen here because we should filter before we get
		// in here, but we'll do something reasonable rather than panic.
		return "(not yet known)"
	case ty == cty.Bool:
		if val.True() {
			return "true"
		}
		return "false"
	case ty == cty.Number:
		bf := val.AsBigFloat()
		return bf.Text('g', 10)
	case ty == cty.String:
		// Go string syntax is not exactly the same as HCL native string syntax,
		// but we'll accept the minor edge-cases where this is different here
		// for now, just to get something reasonable here.
		return fmt.Sprintf("%q", val.AsString())
	case ty.IsCollectionType() || ty.IsTupleType():
		l := val.LengthInt()
		switch l {
		case 0:
			return "empty " + ty.FriendlyName()
		case 1:
			return ty.FriendlyName() + " with 1 element"
		default:
			return fmt.Sprintf("%s with %d elements", ty.FriendlyName(), l)
		}
	case ty.IsObjectType():
		atys := ty.AttributeTypes()
		l := len(atys)
		switch l {
		case 0:
			return "object with no attributes"
		case 1:
			var name string
			for k := range atys {
				name = k
			}
			return fmt.Sprintf("object with 1 attribute %q", name)
		default:
			return fmt.Sprintf("object with %d attributes", l)
		}
	default:
		return ty.FriendlyName()
	}
}

// traversalStr produces a representation of an HCL traversal that is compact,
// resembles HCL native syntax, and is suitable for display in the UI.
func traversalStr(traversal hcl.Traversal) string {
	// This is a specialized subset of traversal rendering tailored to
	// producing helpful contextual messages in diagnostics. It is not
	// comprehensive nor intended to be used for other purposes.

	var buf strings.Builder
	for _, step := range traversal {
		switch tStep := step.(type) {
		case hcl.TraverseRoot:
			buf.WriteString(tStep.Name)
		case hcl.TraverseAttr:
			buf.WriteByte('.')
			buf.WriteString(tStep.Name)
		case hcl.TraverseIndex:
			buf.WriteByte('[')
			if keyTy := tStep.Key.Type(); keyTy.IsPrimitiveType() {
				buf.WriteString(compactValueStr(tStep.Key))
			} else {
				// We'll just use a placeholder for more complex values,
				// since otherwise our result could grow ridiculously long.
				buf.WriteString("...")
			}
			buf.WriteByte(']')
		}
	}
	return buf.String()
}

// tryHCLFileWithNavMetadata takes an hcl.File that might have been directly
// constructed rather than produced by an HCL parser, and tries to pass it
// through a suitable HCL parser if it lacks the metadata that an HCL parser
// would normally add.
//
// If parsing would be necessary to produce the metadata but parsing fails
// then this returns the given file verbatim, so the caller must still be
// prepared to deal with a file lacking navigation metadata.
func tryHCLFileWithNavMetadata(file *hcl.File, filename string) *hcl.File {
	if file.Nav != nil {
		// If there's _something_ in this field then we'll assume that
		// an HCL parser put it there. The details of this field are
		// HCL-parser-specific so we don't try to dig any deeper.
		return file
	}

	// If we have a nil nav then we'll try to construct a fully-fledged
	// file by parsing what we were given. This is best-effort, because
	// the file might well have been lacking navigation metadata due to
	// having been invalid in the first place.
	// Re-parsing a file that might well have already been parsed already
	// earlier is a little wasteful, but we only get here when we're
	// returning diagnostics and so we'd rather do a little extra work
	// if it might allow us to return a better diagnostic.
	parser := hclparse.NewParser()
	var newFile *hcl.File
	if strings.HasSuffix(filename, ".json") {
		newFile, _ = parser.ParseJSON(file.Bytes, filename)
	} else {
		newFile, _ = parser.ParseHCL(file.Bytes, filename)
	}
	if newFile == nil {
		// Our best efforts have failed, then. We'll just return what we had.
		return file
	}
	return newFile
}
