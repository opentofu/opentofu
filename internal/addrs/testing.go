// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty-debug/ctydebug"
)

// CmpOptionsForTesting are a common set of options for
// github.com/google/go-cmp/cmp's [cmp.Diff] that arrange for comparisons to
// ignore irrelevant unexported parts of various addrs types, such as interface
// implementation sigils, which are used for compile-time checks rather than
// runtime checks and so don't need to be compared in our tests.
//
// This also includes the options from go-cty-debug that arrange for [cty.Value]
// and [cty.Type] to be comparable, because those tend to arise as part of
// results from functions in this package.
var CmpOptionsForTesting cmp.Options = cmp.Options{
	// Types that embed [referenceable] as part of their implementations of
	// [Referenceable].
	cmpopts.IgnoreUnexported(
		CountAttr{},
		ForEachAttr{},
		InputVariable{},
		LocalValue{},
		ModuleCallInstance{},
		ModuleCallInstanceOutput{},
		PathAttr{},
		TerraformAttr{},
	),
	// HCL's "Traverser" implementations also use an unexported field to
	// represent their implementation of that interface. We return
	// raw traversals from functions like [ParseRef], so it's helpful
	// to ignore the unexported parts of these too.
	cmpopts.IgnoreUnexported(
		hcl.TraverseAttr{},
		hcl.TraverseIndex{},
		hcl.TraverseRoot{},
	),
	ctydebug.CmpOptions,
}
