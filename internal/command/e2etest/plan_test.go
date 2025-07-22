// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/e2e"
)

// The tests in this file are for the following sequence:
// tofu init
// tofu plan

func TestPlanConsolidatedWarningsForDeprecatedMarks(t *testing.T) {
	t.Parallel()

	implicitFixturePath := filepath.Join("testdata", "consolidated-warnings-for-deprecated-marks")
	tf := e2e.NewBinary(t, tofuBin, implicitFixturePath)

	t.Run("consolidated warnings for deprecated marks", func(t *testing.T) {
		_, initErr, err := tf.Run("init")
		if err != nil {
			t.Fatalf("expected no errors on init, got error %v: %s", err, initErr)
		}

		planStdout, planErr, err := tf.Run("plan")
		if err != nil {
			t.Fatalf("expected no errors on plan, got error %v: %s", err, planErr)
		}

		expectedOutput := `
No changes. Your infrastructure matches the configuration.

OpenTofu has compared your real infrastructure against your configuration and
found no differences, so no changes are needed.
╷
│ Warning: Variable marked as deprecated by the module author
│ 
│   on main.tf line 3, in module "call":
│    3:   input  = "test"
│ 
│ Variable "input" is marked as deprecated with the following message:
│ this is local deprecated
│ 
│ (and one more similar warning elsewhere)
╵
╷
│ Warning: Variable marked as deprecated by the module author
│ 
│   on main.tf line 4, in module "call":
│    4:   input2 = "test2"
│ 
│ Variable "input2" is marked as deprecated with the following message:
│ this is local deprecated2
│ 
│ (and one more similar warning elsewhere)
╵
╷
│ Warning: Value derived from a deprecated source
│ 
│   on main.tf line 14, in locals:
│   14:   i1 = module.call.modout1
│ 
│ This value is derived from module.call.modout1, which is deprecated with
│ the following message:
│ 
│ output deprecated
│ 
│ (and 2 more similar warnings elsewhere)
╵
╷
│ Warning: Value derived from a deprecated source
│ 
│   on main.tf line 15, in locals:
│   15:   i2 = module.call.modout2
│ 
│ This value is derived from module.call.modout2, which is deprecated with
│ the following message:
│ 
│ output deprecated
│ 
│ (and 2 more similar warnings elsewhere)
╵
`
		if diff := cmp.Diff(strings.TrimSpace(stripAnsi(planStdout)), strings.TrimSpace(stripAnsi(expectedOutput))); diff != "" {
			t.Errorf("wrong output.\n%s", diff)
		}
	})
}
