// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package tofu

import "testing"

func TestParseDeprecatedWarningLevel(t *testing.T) {
	t.Run("'all' level parsing", func(t *testing.T) {
		if got, expected := ParseDeprecatedWarningLevel("all"), DeprecationWarningLevelAll; got != expected {
			t.Errorf("parsing %s deprecation level resulted in a wrong value. got: %s; expected: %s", "all", got, expected)
		}
	})
	t.Run("'local' level parsing", func(t *testing.T) {
		if got, expected := ParseDeprecatedWarningLevel("local"), DeprecationWarningLevelLocal; got != expected {
			t.Errorf("parsing %s deprecation level resulted in a wrong value. got: %s; expected: %s", "local", got, expected)
		}
	})
	t.Run("'none' level parsing", func(t *testing.T) {
		if got, expected := ParseDeprecatedWarningLevel("local"), DeprecationWarningLevelLocal; got != expected {
			t.Errorf("parsing %s deprecation level resulted in a wrong value. got: %s; expected: %s", "none", got, expected)
		}
	})
	t.Run("'remote' level parsing", func(t *testing.T) {
		if got, expected := ParseDeprecatedWarningLevel("local"), DeprecationWarningLevelLocal; got != expected {
			t.Errorf("parsing %s deprecation level resulted in a wrong value. got: %s; expected: %s", "remote", got, expected)
		}
	})
	t.Run("'wrongLevel' level parsing", func(t *testing.T) {
		if got, expected := ParseDeprecatedWarningLevel("local"), DeprecationWarningLevelLocal; got != expected {
			t.Errorf("parsing %s deprecation level resulted in a wrong value. got: %s; expected: %s", "wrongLevel", got, expected)
		}
	})
}
