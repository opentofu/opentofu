// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// The plugins package abstracts away many of the details in managing provider and provisioner plugins.
//
// It's primary goal is to provide a library of plugins, who's instances can be managed in different concurrent
// scopes. It also handles many of the complexities around correctly caching plugin schemas.
//
// This package was introduced to solve the following problems:
// * De-duplicate common logic between the original tofu engine and the new engine implementation
// * Re-use this common logic for backends as plugins / PSS
// * Potentially allow plugins to be used by middleware/integrations/etc... (still in the design phase)
// * Properly fix the global schema cache replacement
// * Ensure that provider schema validation is actually called (heavily bugged before)
//
// It's current design intentions are that of a building block, and is not highly opinionated on abstracting
// away the implementation details of plugins.

package plugins
