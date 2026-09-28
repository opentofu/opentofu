// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package plugintest contains helpers for running provider and provisioner
// servers only when testing our plugin client code.
//
// This package must be used only from plugin servers used exclusively during
// testing. In particular, nothing that's linked into the "tofu" executable
// may import this package.
package plugintest
