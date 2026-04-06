// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cloud

import (
	"context"

	tfe "github.com/hashicorp/go-tfe"
)

const (
	changedPolicyEnforcementAction = "changed_policy_enforcements"
	changedTaskEnforcementAction   = "changed_task_enforcements"
	ignoredPolicySetAction         = "ignored_policy_sets"
)

func (b *Cloud) renderRunWarnings(ctx context.Context, client *tfe.Client, runId string) error {
	if b.View == nil {
		return nil
	}

	result, err := client.RunEvents.List(ctx, runId, nil)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}

	// We don't have to worry about paging as the API doesn't support it yet
	for _, re := range result.Items {
		switch re.Action {
		case changedPolicyEnforcementAction, changedTaskEnforcementAction, ignoredPolicySetAction:
			b.View.RunWarning(re.Description)
		}
	}

	return nil
}
