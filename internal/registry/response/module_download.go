// Copyright (c) OpenTofu
// SPDX-License-Identifier: MPL-2.0

package response

import (
	"bytes"
	"fmt"
)

// ModuleLocationRegistryResp defines the OpenTofu registry response
// returned when calling the endpoint /v1/modules/:namespace/:name/:system/:version/download
type ModuleLocationRegistryResp struct {
	// The URL to download the module from.
	Location string `json:"location"`

	// If not nil, represents that the registry wishes to provide the module
	// package directly itself instead of delegating to a separate
	// go-getter-style source address.
	//
	// In that case, the registry can set either "true" to request that the
	// final download request should use the same credentials used to fetch the
	// download location, or "false" to request that the request should be
	// made anonymously (e.g. if the URL already contains something that acts
	// as authentication credentials).
	UseRegistryCredentials *StrictBool `json:"use_registry_credentials"`
}

// StrictBool is a named type representing a bool value that must be written
// in JSON as exactly "true" or "false". In particular, "null" is not permitted
// as an alias for "false", unlike the Go JSON package's default behavior.
//
// This is here really just to implement [json.Unmarshaler] for conveniently
// handling JSON properties that have this requirement.
type StrictBool bool

func (b *StrictBool) UnmarshalJSON(src []byte) error {
	// This method gets called only when the associated JSON property is
	// actually present, and in that case gets called with a preallocated
	// bool value that we need to overwrite based on the source.
	src = bytes.TrimSpace(src)

	// There are only two possible valid JSON boolean tokens, so we'll just
	// handle them directly here for simplicity's sake.
	if bytes.Equal(src, []byte{'t', 'r', 'u', 'e'}) {
		*b = true
	} else if bytes.Equal(src, []byte{'f', 'a', 'l', 's', 'e'}) {
		*b = false
	} else {
		return fmt.Errorf("must be either true or false")
	}
	return nil
}
