// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

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

// AbsProviderConfigCorrect is an experimental "correct" representation of
// the absolute address of a provider configuration block, belonging to
// a module instance.
type AbsProviderConfigCorrect struct {
	Module   ModuleInstance
	Provider Provider
	Alias    string
}

func (pc AbsProviderConfigCorrect) Instance(key InstanceKey) AbsProviderInstanceCorrect {
	return AbsProviderInstanceCorrect{
		Config: pc,
		Key:    key,
	}
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

// The [LocalProviderConfig] type is still reasonable to use to represent
// the special expression syntax we use for referring to provider configs
// in locations like the "provider" meta-argument in a resource configuration.
// As when using it in conjunction with [AbsProviderConfig], it is only
// possible to translate that to an equivalent [AbsProviderConfigCorrect]
// by reference to the provider local name table in the module containing
// the local reference, because each module is allowed to define its own
// mapping from local names to fully-qualified [addrs.Provider].
