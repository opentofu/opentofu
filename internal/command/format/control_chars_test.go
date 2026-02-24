// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package format

import (
	"fmt"
	"testing"
)

func TestFilterControlChars(t *testing.T) {
	tests := map[string]string{
		"Hello, world!":   "Hello, world!",
		"Hello\nworld!":   "Hello\nworld!",
		"Hello\rworld!":   "Hello\rworld!",
		"Hello\r\nworld!": "Hello\r\nworld!",
		"Hello world\x00": "Hello world␀",

		// Filter various ways that someone might try to hide or replace earlier
		// output from OpenTofu.
		"Hello\x7f\x7f\x7f\x7f\x7fGoodbye, world!": "Hello␡␡␡␡␡Goodbye, world!",
		"Hello\x08\x08\x08\x08\x08Goodbye, world!": "Hello␈␈␈␈␈Goodbye, world!",
		"\x1b[1m": "␛[1m", // "Set Graphic Rendition" (SGR) control sequence
		"\x1bM":   "␛M",   // "Reverse Index" (RI) control sequence (moves cursor up, so subsequent text could overwrite earlier text)

		// The cases above ensure that we handle some relatively-likely
		// combinations in a sensible way, but we'll also just exhaustively
		// test all of them together to make sure they all get handled in
		// a reasonable way.
		"\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f \x7f": "␀␁␂␃␄␅␆␇␈\t\n␋␌\r␎␏␐␑␒␓␔␕␖␗␘␙␚␛␜␝␞␟ ␡",
	}

	for input, want := range tests {
		t.Run(fmt.Sprintf("%q", input), func(t *testing.T) {
			got := ReplaceControlChars(input)
			if got != want {
				t.Errorf("wrong result\ninput: %q\ngot:   %q\nwant:  %q", input, got, want)
			}
		})
	}
}
