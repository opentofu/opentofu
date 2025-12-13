// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package classifier

import (
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
)

func TestResourceClassifier_RealScenario(t *testing.T) {
	classifier := NewResourceClassifier()

	plan := &plans.Plan{
		Changes: &plans.Changes{
			Resources: []*plans.ResourceInstanceChangeSrc{
				createResourceChange(t, "aws_s3_bucket.new_bucket", plans.Create),
				createResourceChange(t, "aws_instance.web", plans.Update),
				createResourceChange(t, "aws_instance.old", plans.Delete),
				createResourceChange(t, "aws_instance.replaced", plans.DeleteThenCreate),
			},
		},
	}

	classifications := classifier.ClassifyPlan(plan)

	if len(classifications) != 4 {
		t.Errorf("Expected 4 classifications, got %d", len(classifications))
	}

	counts := classifier.CountBySafetyLevel(plan)
	if counts[SafetySafe] != 1 {
		t.Errorf("Expected 1 safe change, got %d", counts[SafetySafe])
	}
	if counts[SafetyRisky] != 1 {
		t.Errorf("Expected 1 risky change, got %d", counts[SafetyRisky])
	}
	if counts[SafetyDestructive] != 2 {
		t.Errorf("Expected 2 destructive changes, got %d", counts[SafetyDestructive])
	}

	if classifier.HasOnlySafeChanges(plan) {
		t.Error("Expected HasOnlySafeChanges to be false")
	}

	if !classifier.HasDestructiveChanges(plan) {
		t.Error("Expected HasDestructiveChanges to be true")
	}
}

func TestResourceClassifier_SafeOnlyPlan(t *testing.T) {
	classifier := NewResourceClassifier()

	plan := &plans.Plan{
		Changes: &plans.Changes{
			Resources: []*plans.ResourceInstanceChangeSrc{
				createResourceChange(t, "aws_s3_bucket.new1", plans.Create),
				createResourceChange(t, "aws_s3_bucket.new2", plans.Create),
				createResourceChange(t, "data.aws_ami.latest", plans.Read),
			},
		},
	}

	counts := classifier.CountBySafetyLevel(plan)
	if counts[SafetySafe] != 3 {
		t.Errorf("Expected 3 safe changes, got %d", counts[SafetySafe])
	}

	if !classifier.HasOnlySafeChanges(plan) {
		t.Error("Expected HasOnlySafeChanges to be true for safe-only plan")
	}

	if classifier.HasDestructiveChanges(plan) {
		t.Error("Expected HasDestructiveChanges to be false for safe-only plan")
	}
}

func TestResourceClassifier_EmptyPlan(t *testing.T) {
	classifier := NewResourceClassifier()

	plan := &plans.Plan{
		Changes: &plans.Changes{
			Resources: []*plans.ResourceInstanceChangeSrc{},
		},
	}

	counts := classifier.CountBySafetyLevel(plan)
	total := counts[SafetySafe] + counts[SafetyRisky] + counts[SafetyDestructive] + counts[SafetyUnknown]
	if total != 0 {
		t.Errorf("Expected 0 total changes, got %d", total)
	}

	if !classifier.HasOnlySafeChanges(plan) {
		t.Error("Expected empty plan to be considered safe")
	}
}

func TestResourceClassifier_NilPlan(t *testing.T) {
	classifier := NewResourceClassifier()

	classifications := classifier.ClassifyPlan(nil)
	if len(classifications) != 0 {
		t.Errorf("Expected 0 classifications for nil plan, got %d", len(classifications))
	}

	counts := classifier.CountBySafetyLevel(nil)
	total := counts[SafetySafe] + counts[SafetyRisky] + counts[SafetyDestructive] + counts[SafetyUnknown]
	if total != 0 {
		t.Errorf("Expected 0 total changes for nil plan, got %d", total)
	}
}

func createResourceChange(t *testing.T, addrStr string, action plans.Action) *plans.ResourceInstanceChangeSrc {
	t.Helper()

	addr, diags := addrs.ParseAbsResourceInstanceStr(addrStr)
	if diags.HasErrors() {
		t.Fatalf("Failed to parse address %s: %s", addrStr, diags.Err())
	}

	return &plans.ResourceInstanceChangeSrc{
		Addr: addr,
		ChangeSrc: plans.ChangeSrc{
			Action: action,
		},
		ProviderAddr: addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.MustParseProviderSourceString("hashicorp/aws"),
		},
	}
}

func TestSafetyLevel_String(t *testing.T) {
	tests := []struct {
		level    SafetyLevel
		expected string
	}{
		{SafetyUnknown, "unknown"},
		{SafetySafe, "safe"},
		{SafetyRisky, "risky"},
		{SafetyDestructive, "destructive"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("SafetyLevel.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestChangeClassification(t *testing.T) {
	classification := &ChangeClassification{
		SafetyLevel: SafetyDestructive,
		Reason:      "resource_deletion",
		Description: "Удаление ресурса необратимо",
	}

	if classification.SafetyLevel != SafetyDestructive {
		t.Errorf("Expected SafetyDestructive, got %v", classification.SafetyLevel)
	}

	if classification.Reason != "resource_deletion" {
		t.Errorf("Expected 'resource_deletion', got %s", classification.Reason)
	}

	if classification.Description == "" {
		t.Error("Description should not be empty")
	}
}
