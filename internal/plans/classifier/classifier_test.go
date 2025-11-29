// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package classifier

import (
	"testing"

	"github.com/opentofu/opentofu/internal/plans"
)

func TestClassifier_AllActions(t *testing.T) {
	classifier := NewClassifier()

	testCases := []struct {
		action        plans.Action
		expectedLevel SafetyLevel
		name          string
	}{
		{plans.Delete, SafetyDestructive, "Delete"},
		{plans.DeleteThenCreate, SafetyDestructive, "DeleteThenCreate"},
		{plans.CreateThenDelete, SafetyDestructive, "CreateThenDelete"},
		{plans.Create, SafetySafe, "Create"},
		{plans.Read, SafetySafe, "Read"},
		{plans.NoOp, SafetySafe, "NoOp"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			classification := classifier.ClassifyByAction(tc.action)
			if classification.SafetyLevel != tc.expectedLevel {
				t.Errorf("%s: expected %v, got %v", tc.name, tc.expectedLevel, classification.SafetyLevel)
			}
		})
	}
}

func TestIsDestructiveAction(t *testing.T) {
	classifier := NewClassifier()

	destructiveActions := []plans.Action{
		plans.Delete,
		plans.DeleteThenCreate,
		plans.CreateThenDelete,
	}

	for _, action := range destructiveActions {
		if !classifier.IsDestructiveAction(action) {
			t.Errorf("Action %s should be destructive", action.String())
		}
	}

	safeActions := []plans.Action{
		plans.Create,
		plans.Update,
		plans.Read,
		plans.NoOp,
	}

	for _, action := range safeActions {
		if classifier.IsDestructiveAction(action) {
			t.Errorf("Action %s should not be destructive", action.String())
		}
	}
}

func TestIntegration(t *testing.T) {
	classifier := NewClassifier()

	actions := []plans.Action{
		plans.Create,
		plans.Update,
		plans.Delete,
		plans.NoOp,
	}

	for _, action := range actions {
		classification := classifier.ClassifyByAction(action)
		if classification == nil {
			t.Errorf("Classification for %s should not be nil", action.String())
			continue
		}
		t.Logf("Action: %s -> Safety: %v, Reason: %s",
			action.String(), classification.SafetyLevel, classification.Reason)
	}
}
