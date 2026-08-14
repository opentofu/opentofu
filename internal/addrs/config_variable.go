// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
)

// ConfigVariable represents the physical address of a variable inside the configuration
// by storing the name of the variable and its declaration range.
type ConfigVariable struct {
	Variable  InputVariable
	DeclRange hcl.Range
}

func (v ConfigVariable) String() string {
	return "var." + v.Variable.Name
}

func (v ConfigVariable) UniqueKey() UniqueKey {
	raw := fmt.Sprintf("%s-%s", v.Variable.Name, v.DeclRange.String())
	return configVariableUniqueKey(raw)
}

func (v ConfigVariable) uniqueKeySigil() {}

type configVariableUniqueKey string

func (k configVariableUniqueKey) uniqueKeySigil() {}

// ConfigLocal represents the physical address of a local inside the configuration
// by storing the name of the local and its declaration range.
type ConfigLocal struct {
	Local     LocalValue
	DeclRange hcl.Range
}

func (v ConfigLocal) String() string {
	return "local." + v.Local.Name
}

func (v ConfigLocal) UniqueKey() UniqueKey {
	raw := fmt.Sprintf("%s-%s", v.Local.Name, v.DeclRange.String())
	return configLocalUniqueKey(raw)
}

func (v ConfigLocal) uniqueKeySigil() {}

type configLocalUniqueKey string

func (k configLocalUniqueKey) uniqueKeySigil() {}
