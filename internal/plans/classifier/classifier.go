// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package classifier

import (
	"github.com/opentofu/opentofu/internal/plans"
)

type SafetyLevel int

const (
	SafetyUnknown SafetyLevel = iota
	SafetySafe
	SafetyRisky
	SafetyDestructive
)

func (s SafetyLevel) String() string {
	switch s {
	case SafetySafe:
		return "safe"
	case SafetyRisky:
		return "risky"
	case SafetyDestructive:
		return "destructive"
	default:
		return "unknown"
	}
}

type ChangeClassification struct {
	SafetyLevel SafetyLevel
	Reason      string
	Description string
}

type Classifier struct{}

func NewClassifier() *Classifier {
	return &Classifier{}
}

func (c *Classifier) ClassifyByAction(action plans.Action) *ChangeClassification {
	switch action {
	case plans.Delete:
		return &ChangeClassification{
			SafetyLevel: SafetyDestructive,
			Reason:      "resource_deletion",
			Description: "Resource deletion is irreversible",
		}
	case plans.DeleteThenCreate:
		return &ChangeClassification{
			SafetyLevel: SafetyDestructive,
			Reason:      "delete_then_create",
			Description: "Deleting and creating a resource is a destructive operation",
		}
	case plans.CreateThenDelete:
		return &ChangeClassification{
			SafetyLevel: SafetyDestructive,
			Reason:      "create_then_delete",
			Description: "Creating and deleting a resource is a destructive operation",
		}
	case plans.Create:
		return &ChangeClassification{
			SafetyLevel: SafetySafe,
			Reason:      "resource_creation",
			Description: "Creating a new resource is a safe operation",
		}
	case plans.Update:
		return &ChangeClassification{
			SafetyLevel: SafetyRisky,
			Reason:      "resource_update",
			Description: "Updating a resource requires checking parameters",
		}
	case plans.Read:
		return &ChangeClassification{
			SafetyLevel: SafetySafe,
			Reason:      "read_only",
			Description: "Read-only operation",
		}
	case plans.NoOp:
		return &ChangeClassification{
			SafetyLevel: SafetySafe,
			Reason:      "no_operation",
			Description: "No changes required",
		}
	default:
		return &ChangeClassification{
			SafetyLevel: SafetyUnknown,
			Reason:      "unknown_action",
			Description: "Unknown action type: " + action.String(),
		}
	}
}

func (c *Classifier) ClassifyPlan(plan *plans.Plan) SafetyLevel {
	if plan == nil || len(plan.Changes.Resources) == 0 {
		return SafetySafe
	}

	worstLevel := SafetySafe
	for _, change := range plan.Changes.Resources {
		classification := c.ClassifyByAction(change.Action)
		if classification.SafetyLevel > worstLevel {
			worstLevel = classification.SafetyLevel
		}
	}
	return worstLevel
}

func (c *Classifier) IsDestructiveAction(action plans.Action) bool {
	return action == plans.Delete ||
		action == plans.DeleteThenCreate ||
		action == plans.CreateThenDelete
}

func (c *Classifier) IsRiskyAction(action plans.Action) bool {
	return action == plans.Update
}

func (c *Classifier) IsSafeAction(action plans.Action) bool {
	return action == plans.Create || action == plans.Read || action == plans.NoOp
}
