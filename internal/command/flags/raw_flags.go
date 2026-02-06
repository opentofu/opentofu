// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package flags

import (
	"flag"
	"fmt"
)

// RawFlags is a flag.Value implementation that appends raw flag
// names and values to a slice. This is used to collect a sequence of flags
// with possibly different names, preserving the overall order.
type RawFlags struct {
	Name  string
	Items *[]RawFlag
}

var _ flag.Value = RawFlags{}

func NewRawFlags(name string) RawFlags {
	var items []RawFlag
	return RawFlags{
		Name:  name,
		Items: &items,
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
		Name:  flagName,
		Items: f.Items,
	}
}

func (f RawFlags) String() string {
	return ""
}

func (f RawFlags) Set(str string) error {
	*f.Items = append(*f.Items, RawFlag{
		Name:  f.Name,
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
