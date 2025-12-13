// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package classifier

import (
	"github.com/opentofu/opentofu/internal/plans"
)

type ResourceClassifier struct {
	*Classifier
}

func NewResourceClassifier() *ResourceClassifier {
	return &ResourceClassifier{
		Classifier: NewClassifier(),
	}
}

func (rc *ResourceClassifier) ClassifyResourceChange(change *plans.ResourceInstanceChangeSrc) *ChangeClassification {
	classification := rc.ClassifyByAction(change.ChangeSrc.Action)

	if classification != nil && change.Addr.Resource.Resource.Type == "local_file" {
		if change.ChangeSrc.Action == plans.Create {
			return &ChangeClassification{
				SafetyLevel: SafetySafe,
				Reason:      "safe_local_creation",
				Description: "Создание локального файла - безопасная операция",
			}
		}
	}

	return classification
}

func (rc *ResourceClassifier) ClassifyPlan(plan *plans.Plan) map[string]*ChangeClassification {
	classifications := make(map[string]*ChangeClassification)

	if plan == nil || plan.Changes == nil {
		return classifications
	}

	for _, change := range plan.Changes.Resources {
		classification := rc.ClassifyResourceChange(change)
		if classification != nil {
			classifications[change.Addr.String()] = classification
		}
	}

	return classifications
}

func (rc *ResourceClassifier) CountBySafetyLevel(plan *plans.Plan) map[SafetyLevel]int {
	counts := map[SafetyLevel]int{
		SafetyUnknown:     0,
		SafetySafe:        0,
		SafetyRisky:       0,
		SafetyDestructive: 0,
	}

	classifications := rc.ClassifyPlan(plan)
	for _, classification := range classifications {
		counts[classification.SafetyLevel]++
	}

	return counts
}

func (rc *ResourceClassifier) HasDestructiveChanges(plan *plans.Plan) bool {
	counts := rc.CountBySafetyLevel(plan)
	return counts[SafetyDestructive] > 0
}

func (rc *ResourceClassifier) HasOnlySafeChanges(plan *plans.Plan) bool {
	counts := rc.CountBySafetyLevel(plan)
	return counts[SafetyRisky] == 0 && counts[SafetyDestructive] == 0 && counts[SafetyUnknown] == 0
}
