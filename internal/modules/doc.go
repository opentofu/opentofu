// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package modules is a sketch of a potential new approach for the early
// work currently done in "package configs", which makes some different
// decisions about how the different concerns are separated.
//
// The general idea here is that "package modules" deals with the lower-level
// details of finding all of the source files in a module, parsing them,
// and decoding their static structural elements, but then the work of creating
// the final "configuration tree" for the rest of OpenTofu to use is done
// elsewhere -- perhaps still in "package configs"? -- returning a different set
// of types where all early-evaluated values are in place.
//
// This is not intended to ever be merged as-is, and is just dead code intended
// for humans to read and think about. If we do decide to do something like this
// at a later point, we'll presumably evolve "package configs" gradually into
// this shape rather than completely starting over.
package modules
