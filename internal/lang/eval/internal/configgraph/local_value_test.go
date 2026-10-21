// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
)

func TestLocalValue_Value(t *testing.T) {
	testAddr := addrs.LocalValue{Name: "test"}
	testValuer(t, map[string]valuerTest[*LocalValue]{
		"basics": {
			&LocalValue{
				Addr:     testAddr.Absolute(addrs.RootModuleInstance),
				RawValue: constantOnceValuer(cty.StringVal("Hello, world!"), "RawValue %s", testAddr),
			},
			cty.StringVal("Hello, world!"),
			nil,
		},
	})
}
