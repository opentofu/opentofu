// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ssh

import (
	"reflect"
	"testing"
)

func TestPasswordKeyboardInteractive_Challenge(t *testing.T) {
	p := PasswordKeyboardInteractive("foo")
	result, err := p("foo", "bar", []string{"one", "two"}, nil)
	if err != nil {
		t.Fatalf("err not nil: %s", err)
	}

	if !reflect.DeepEqual(result, []string{"foo", "foo"}) {
		t.Fatalf("invalid password: %#v", result)
	}
}
