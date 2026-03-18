// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"github.com/zclconf/go-cty/cty"
)

// ValueWithPrivate is a [cty.Value] associated with an arbitrary "private"
// byte array that is somehow related to it.
//
// Refer to the documentation of any field or argument using this type to
// learn how the value and the byte array are related. Typically this is used
// to allow a provider to transfer some additional out-of-band information
// alongside a value that it will use when the same value is resubmitted to
// the same provider later.
type ValueWithPrivate struct {
	Value   cty.Value
	Private []byte
}
