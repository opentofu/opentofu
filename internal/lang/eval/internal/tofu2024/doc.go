// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package tofu2024 contains the "module compiler" implementation for the
// first edition of the OpenTofu language, established with OpenTofu v1.6
// in 2024 and then gradually evolved in backward-compatible ways.
//
// This package owns the responsibility of translating from [configs.Module]
// to the language-edition-agnostic API used by packages eval configgraph.
//
// Conceptually then, this package decides the meaning of and relationships
// between blocks and expressions in an OpenTofu module that is written for
// this language edition, which is the default edition used when no other
// edition is selected. (At the time of initially writing this doc there
// are no other editions available, but this doc text is hedging in case
// we've forgotten to update it after adding one or more later editions.)
package tofu2024

// the following imports are only for the links in the doc comment above
import (
	_ "github.com/opentofu/opentofu/internal/configs"
)

// === SOME HISTORICAL NOTES ===
//
// For those who are coming here with familiarity with the original runtime
// in "package tofu", you might like to think of the types in this package as
// being _roughly_ analogous to the "graph builder" mechanism in package tofu.
//
// There are some notable differences that are worth knowing before you dive
// in here, though:
//
// - The "compile" code here is intentionally written as much as possible as
//   straight-through code that runs to completion and returns a value, whereas
//   package tofu's graph builders instead follow an inversion-of-control style
//   where a bunch of transformers are run sequentially and each make arbitrary
//   modifications to a shared mutable data structure.
// - The "graph" that this code is building is based on the types in the sibling
//   package "configgraph", which at the time of writing has its own "historical
//   notes" like this describing how it relates to the traditional graph model.
// - An express goal of this "compiler" layer is to create an abstraction
//   boundary between the current surface language, presently implemented in
//   "package configs", and the execution engine which ideally cares only about
//   the relationships between objects and the values flowing between them.
//   Therefore nothing in package configgraph should depend on anything from
//   package configs, and configgraph should also only be using HCL directly for
//   some ancillary concepts like diagnostics and traversals, and even those
//   maybe we'll replace with some OpenTofu-specific wrapper types in future.

// Temporary note about possible future plans:
//
// Currently this package is working with [configs.Module] and the other types
// that appear nested within it so that we don't need to rewrite the config
// decoding logic at the same time as replacing the evaluator, but we've
// talked about moving to a model where the first level of config decoding
// is concerned only with the top-level structure -- finding the relevant
// files, collecting the top-level [hcl.Block] from them and applying the
// merging/overriding rules -- and would no longer do any deeper decoding
// of the _content_ of those top-level declarations.
//
// If we adopted that model then this package is the likely place for the
// deeper decoding to happen. Therefore a reasonable way to think about the
// abstraction this package is providing is that ideally we should be able
// to switch away from [configs] to whatever replaces it only by changing
// the compilation logic in _this_ package, while preserving the abstraction
// so that all of the subsequent steps don't need to be modified at all.
//
// That is in contrast to the previous situation with "package tofu", where
// the execution logic is tightly coupled with various [configs] types and
// so it's hard to make changes to how we model the first level of decoding
// without significant disruptions to the runtime and its tests.
