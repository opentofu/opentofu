// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package tofu

import (
	"fmt"
	"testing"
)

func TestParseDeprecatedWarningLevel(t *testing.T) {
	tests := []struct {
		in   string
		want DeprecationWarningLevel
	}{
		{want: DeprecationWarningLevelAll, in: ""},
		{want: DeprecationWarningLevelAll, in: "all"},
		{want: DeprecationWarningLevelLocal, in: "local"},
		{want: DeprecationWarningLevelAll, in: "none"},
		{want: DeprecationWarningLevelAll, in: "off"},
		{want: DeprecationWarningLevelAll, in: "remote"},
		{want: DeprecationWarningLevelAll, in: "wrongLevel"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q level parsing", tt.in), func(t *testing.T) {
			if got, want := ParseDeprecatedWarningLevel(tt.in), tt.want; got != want {
				t.Errorf("parsing %s deprecation level resulted in a wrong value. got: %s; want: %s", tt.in, got, want)
			}
		})
	}

}
