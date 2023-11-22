// Copyright (c) OpenTofu
// SPDX-License-Identifier: MPL-2.0

package response

// ModuleLocationRegistryResp defines the OpenTofu registry response
// returned when calling the endpoint /v1/modules/:namespace/:name/:system/:version/download
type ModuleLocationRegistryResp struct {
	// The URL to download the module from.
	Location string `json:"location"`
}
