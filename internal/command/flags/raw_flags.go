// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package flags

import "fmt"

// RawFlags is a flag.Value implementation that just appends raw flag
// names and values to a slice.
type RawFlags struct {
	FlagName string
	Items    *[]RawFlag
}

func NewRawFlags(flagName string) RawFlags {
	var items []RawFlag
	return RawFlags{
		FlagName: flagName,
		Items:    &items,
	}
}

func (f RawFlags) Empty() bool {
	if f.Items == nil {
		return true
	}
	return len(*f.Items) == 0
}

func (f RawFlags) AllItems() []RawFlag {
	if f.Items == nil {
		return nil
	}
	return *f.Items
}

func (f RawFlags) Alias(flagName string) RawFlags {
	return RawFlags{
		FlagName: flagName,
		Items:    f.Items,
	}
}

func (f RawFlags) String() string {
	return ""
}

func (f RawFlags) Set(str string) error {
	*f.Items = append(*f.Items, RawFlag{
		Name:  f.FlagName,
		Value: str,
	})
	return nil
}

type RawFlag struct {
	Name  string
	Value string
}

func (f RawFlag) String() string {
	return fmt.Sprintf("%s=%q", f.Name, f.Value)
}
