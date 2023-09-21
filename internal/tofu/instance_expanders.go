// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

// graphNodeExpandsInstances is implemented by nodes that causes instances to
// be registered in the instances.Expander.
type graphNodeExpandsInstances interface {
	expandsInstances()
}
