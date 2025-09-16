// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"strings"
)

// This file contains some provider address types that are arguably more
// "correct" than the ones used in most of the system due to following the
// same naming conventions used elsewhere in package addrs.
//
// The types without the "Correct" suffix are all a little confusing for
// various historical reasons related to how providers are treated in special,
// strange ways throughout OpenTofu.
//
// The types in this file are currently used only by experimental new code in
// packages like lang/eval. If you are working on the current code in
// "package tofu"/etc then the types in this file are not for you yet.

type ProviderConfigCorrect struct {
	Provider Provider
	Alias    string
}

func (p ProviderConfigCorrect) Equal(other ProviderConfigCorrect) bool {
	return p.Provider.Equals(other.Provider) && p.Alias == other.Alias
}

// AbsProviderConfigCorrect is an experimental "correct" representation of
// the absolute address of a provider configuration block, belonging to
// a module instance.
type AbsProviderConfigCorrect struct {
	Module ModuleInstance
	Config ProviderConfigCorrect
}

func (pc AbsProviderConfigCorrect) Equal(other AbsProviderConfigCorrect) bool {
	return pc.Module.Equal(other.Module) && pc.Config.Equal(other.Config)
}

func (pc AbsProviderConfigCorrect) Instance(key InstanceKey) AbsProviderInstanceCorrect {
	return AbsProviderInstanceCorrect{
		Config: pc,
		Key:    key,
	}
}

func (pc AbsProviderConfigCorrect) String() string {
	var buf strings.Builder
	if !pc.Module.IsRoot() {
		buf.WriteString(pc.Module.String())
		buf.WriteByte('.')
	}
	fmt.Fprintf(&buf, "provider[%q]", pc.Config.Provider)
	if pc.Config.Alias != "" {
		buf.WriteByte('.')
		buf.WriteString(pc.Config.Alias)
	}
	return buf.String()
}

func (pc AbsProviderConfig) Correct() AbsProviderConfigCorrect {
	return AbsProviderConfigCorrect{
		Module: pc.Module.UnkeyedInstanceShim(),
		Config: ProviderConfigCorrect{
			Provider: pc.Provider,
			Alias:    pc.Alias,
		},
	}
}

type ProviderInstanceCorrect struct {
	Config ProviderConfigCorrect
	Key    InstanceKey
}

// AbsProviderInstanceCorrect is an experimental "correct" representation of
// the absolute address of an instance of an [AbsProviderConfigCorrect].
//
// A provider configuration block has zero or more instances, based on whether
// it uses the "for_each" meta-argument and, if so, what it's set to.
type AbsProviderInstanceCorrect struct {
	Config AbsProviderConfigCorrect
	Key    InstanceKey
}

func (a AbsProviderInstanceCorrect) Equal(other AbsProviderInstanceCorrect) bool {
	return a.Config.Equal(other.Config) && a.Key == other.Key
}

func (pc AbsProviderConfig) InstanceCorrect(key InstanceKey) AbsProviderInstanceCorrect {
	return AbsProviderInstanceCorrect{
		Config: pc.Correct(),
		Key:    key,
	}
}

var _ UniqueKeyer = AbsProviderInstanceCorrect{}

func (a AbsProviderInstanceCorrect) String() string {
	if a.Key == nil {
		return a.Config.String()
	}
	return a.Config.String() + a.Key.String()
}

// UniqueKey implements UniqueKeyer.
func (a AbsProviderInstanceCorrect) UniqueKey() UniqueKey {
	return absProviderInstanceCorrectKey(a.String())
}

func (a AbsProviderInstanceCorrect) LocalConfig() ProviderInstanceCorrect {
	return ProviderInstanceCorrect{
		Config: a.Config.Config,
		Key:    a.Key,
	}
}

type absProviderInstanceCorrectKey string

// uniqueKeySigil implements UniqueKey.
func (a absProviderInstanceCorrectKey) uniqueKeySigil() {}

// The [LocalProviderConfig] type is still reasonable to use to represent
// the special expression syntax we use for referring to provider configs
// in locations like the "provider" meta-argument in a resource configuration.
// As when using it in conjunction with [AbsProviderConfig], it is only
// possible to translate that to an equivalent [AbsProviderConfigCorrect]
// by reference to the provider local name table in the module containing
// the local reference, because each module is allowed to define its own
// mapping from local names to fully-qualified [addrs.Provider].
