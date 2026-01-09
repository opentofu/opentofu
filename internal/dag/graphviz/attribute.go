// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graphviz

import (
	"bufio"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type Attributes = map[string]Value

func writeGraphvizAttrList(a Attributes, w *bufio.Writer) error {
	var err error
	// We'll first sort the attribute names so that our output is deterministic
	// for easier unit testing.
	names := slices.Collect(maps.Keys(a))
	slices.Sort(names)
	for i, name := range names {
		val := a[name]

		if i != 0 {
			err = w.WriteByte(',')
			if err != nil {
				return err
			}
		}
		err = writeGraphvizAttr(name, val, w)
		if err != nil {
			return err
		}
	}
	return nil
}

func writeGraphvizAttr(name string, val Value, w *bufio.Writer) error {
	var err error
	_, err = w.WriteString(quoteForGraphviz(name))
	if err != nil {
		return err
	}
	err = w.WriteByte('=')
	if err != nil {
		return err
	}
	_, err = w.WriteString(val.asAttributeValue())
	if err != nil {
		return err
	}
	return nil
}

type Value interface {
	asAttributeValue() string
}

// Val converts its argument into a [Value], for use in [Attributes].
func Val[T interface {
	string | int | PrequotedValue | HTMLLikeString
}](from T) Value {
	// We use generics here just for the ergonomics of being able to pass
	// a plain string or int and then having this function convert it to
	// the corresponding named types, since we cannot add methods to the
	// primitive types.
	switch from := any(from).(type) {
	case string:
		return stringValue(from)
	case int:
		return stringValue(strconv.Itoa(from))
	case HTMLLikeString:
		return from
	case PrequotedValue:
		return from
	default:
		panic("unreachable")
	}
}

type stringValue string

func (s stringValue) asAttributeValue() string {
	return quoteForGraphviz(string(s))
}

// PrequotedValue is a string containing something that should be directly
// inserted into the output without any further processing, because it's
// already been prepared to be valid.
//
// Using this is unfortunately necessary when the caller needs to generate
// values containing Graphviz's special extra escape sequences, for which there
// is no equivalent in Go.
//
// The value will be included verbatim into output without any further
// processing, so if an invalid value is provided then the output will
// not be valid Graphviz language syntax.
type PrequotedValue string

func (s PrequotedValue) asAttributeValue() string {
	return string(s)
}

// HTMLLikeString represents a string that conforms to Graphviz's idea of
// "HTML strings", which in practice use an XML-like grammar to represent a
// language that is _roughly_ a subset of HTML, but also includes
// Graphviz-specific extensions.
//
// A string should be converted to this type only if it contains what Graphviz
// would consider to be valid "HTML string" syntax:
//
//	https://graphviz.org/doc/info/lang.html#html-strings
//
// In practice the value will be included verbatim into output without any
// further processing, so if an invalid value is provided then the output will
// not be valid Graphviz language syntax.
type HTMLLikeString string

func (s HTMLLikeString) asAttributeValue() string {
	// Graphviz distinguishes regular strings from HTML strings by having them
	// enclosed in a set of XML-like brackets. These are in addition to the
	// similar brackets that would appear in the tags inside the value.
	return "<" + string(s) + ">"
}

func quoteForGraphviz(s string) string {
	// We'll leave strings unquoted and unescaped when possible, for
	// better human readability of the output. We force certain strings
	// to be quoted because on some contexts Graphviz gives them special
	// meaning.
	if validUnquoteID.MatchString(s) && s != "node" && s != "edge" {
		return s
	}
	var buf strings.Builder
	buf.WriteByte('"')
	for _, c := range s {
		switch c {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		default:
			buf.WriteRune(c)
		}
	}
	buf.WriteByte('"')
	return buf.String()
}

var validUnquoteID = regexp.MustCompile(`^[a-zA-Z\200-\377_][a-zA-Z0-9\200-\377_]*$`)
