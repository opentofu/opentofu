// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import "fmt"

// TestRunOutputRef is the address of a test run block output.
type TestRunOutputRef struct {
	referenceable
	Name         string
	RunBlockName string
}

func (v TestRunOutputRef) String() string {
	return fmt.Sprintf("run.%s.%s", v.RunBlockName, v.Name)
}

func (v TestRunOutputRef) UniqueKey() UniqueKey {
	return v
}

func (v TestRunOutputRef) uniqueKeySigil() {}
