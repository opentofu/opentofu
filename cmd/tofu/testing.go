// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

// e2eTestingFeatures can be set to any non-empty string using Go linker
// arguments in order to enable the use of e2e test features for a
// particular OpenTofu build:
//
//	go install -ldflags="-X 'main.e2eTestingFeatures=yes'" ./cmd/tofu
//
// By default this variable is initialized as empty, in which case
// e2e testing features are not available.
var e2eTestingFeatures string

func e2eTestingFeaturesEnabled() bool {
	return e2eTestingFeatures != ""
}
