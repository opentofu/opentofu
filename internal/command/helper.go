// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"github.com/terramate-io/opentofulib/internal/backend"
	"github.com/terramate-io/opentofulib/internal/cloud"
)

const failedToLoadSchemasMessage = `
Warning: Failed to update data for external integrations

OpenTofu was unable to generate a description of the updated
state for use with external integrations in the cloud backend.
Any integrations configured for this workspace which depend on
information from the state may not work correctly when using the
result of this action.

This problem occurs when OpenTofu cannot read the schema for
one or more of the providers used in the state. The next successful
apply will correct the problem by re-generating the JSON description
of the state:
    tofu apply
`

func isCloudMode(b backend.Enhanced) bool {
	_, ok := b.(*cloud.Cloud)

	return ok
}
