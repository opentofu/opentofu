// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testhelpers

import (
	"github.com/opentofu/opentofu/internal/provisioners"
)

// simpletesthelpers.MockProvisioner returns a MockProvisioner that is pre-configured
// with schema for its own config, with the same content as returned by
// function simpleTestSchema.
//
// For most reasonable uses the returned provisioner must be registered in a
// componentFactory under the name "test". Use simpleMockComponentFactory
// to obtain a pre-configured componentFactory containing the result of
// this function along with simpletesthelpers.MockProvider, both registered as "test".
//
// The returned provisioner has no other behaviors by default, but the caller
// may modify it in order to stub any other required functionality, or modify
// the default schema stored in the field GetSchemaReturn. Each new call to
// simpleTestProvisioner produces entirely new instances of all of the nested
// objects so that callers can mutate without affecting mock objects.
func SimpleMockProvisioner() *MockProvisioner {
	return &MockProvisioner{
		GetSchemaResponse: provisioners.GetSchemaResponse{
			Provisioner: SimpleTestSchema(),
		},
	}
}
