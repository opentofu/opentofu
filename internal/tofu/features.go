// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import "os"

// This file holds feature flags for the next release

var flagWarnOutputErrors = os.Getenv("TF_WARN_OUTPUT_ERRORS") != ""
