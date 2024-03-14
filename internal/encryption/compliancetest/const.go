// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package compliancetest

import (
	"regexp"
)

var hclTagRe = regexp.MustCompile("^[a-zA-Z0-9_-]+$")
