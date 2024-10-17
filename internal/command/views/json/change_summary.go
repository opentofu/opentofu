// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package json

import (
	"fmt"
	"strings"
)

type Operation string

const (
	OperationApplied   Operation = "apply"
	OperationDestroyed Operation = "destroy"
	OperationPlanned   Operation = "plan"
)

type ChangeSummary struct {
	Add       int       `json:"add"`
	Change    int       `json:"change"`
	Import    int       `json:"import"`
	Remove    int       `json:"remove"`
	Forget    int       `json:"forget"`
	Operation Operation `json:"operation"`
}

// The summary strings for apply and plan are accidentally a public interface
// used by Terraform Cloud and Terraform Enterprise, so the exact formats of
// these strings are important.
func (cs *ChangeSummary) String() string {
	var builder strings.Builder
	switch cs.Operation {
	case OperationApplied:
		builder.WriteString("Apply complete! Resources: ")
		if cs.Import > 0 {
			builder.WriteString(fmt.Sprintf("%d imported, ", cs.Import))
		}
		builder.WriteString(fmt.Sprintf("%d added, %d changed, %d destroyed", cs.Add, cs.Change, cs.Remove))
		if cs.Forget > 0 {
			builder.WriteString(fmt.Sprintf(", %d forgotten.", cs.Forget))
		} else {
			builder.WriteString(".")
		}
		return builder.String()
	case OperationDestroyed:
		return fmt.Sprintf("Destroy complete! Resources: %d destroyed.", cs.Remove)
	case OperationPlanned:
		builder.WriteString("Plan: ")
		if cs.Import > 0 {
			builder.WriteString(fmt.Sprintf("%d to import, ", cs.Import))
		}
		builder.WriteString(fmt.Sprintf("%d to add, %d to change, %d to destroy", cs.Add, cs.Change, cs.Remove))
		if cs.Forget > 0 {
			builder.WriteString(fmt.Sprintf(", %d to forget.", cs.Forget))
		} else {
			builder.WriteString(".")
		}
		return builder.String()
	default:
		return fmt.Sprintf("%s: %d add, %d change, %d destroy", cs.Operation, cs.Add, cs.Change, cs.Remove)
	}
}
