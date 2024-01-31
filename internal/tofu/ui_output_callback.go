// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

type CallbackUIOutput struct {
	OutputFn func(string)
}

func (o *CallbackUIOutput) Output(v string) {
	o.OutputFn(v)
}
