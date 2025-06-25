// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package statestoreshim is a set of temporary shims to help with an initial
// implementation of granular state storage that continues to preserve the
// original assumption that state storage is purely a CLI layer concern and
// that our core language runtime treats state only as an in-memory artifact.
//
// To get the full benefits of granular state storage we would need to integrate
// it more deeply into the language runtime so that e.g. we can fetch individual
// objects on request rather than loading the entire state into RAM before
// doing any other work. But this temporary posture allows us to swap it in
// without disruptive architecture changes just to get some early experience
// with these new APIs.
package statestoreshim
