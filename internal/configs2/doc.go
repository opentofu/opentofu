// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package configs2 is a sketch of a potential new approach for "package configs"
// that makes some different decisions about how the different concerns
// are separated.
//
// This is not intended to ever be merged as-is, and is just dead code intended
// for humans to read and think about. If we do decide to do something like this
// at a later point, we'll presumably evolve "package configs" gradually into
// this shape rather than completely starting over.
package configs2
