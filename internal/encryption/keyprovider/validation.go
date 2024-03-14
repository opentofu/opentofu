// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider

import "regexp"

// TODO is there a generalized way to regexp-check names?
var addrRe = regexp.MustCompile(`^key_provider\.([a-zA-Z_0-9-]+)\.([a-zA-Z_0-9-]+)$`)
var nameRe = regexp.MustCompile("^([a-zA-Z_0-9-]+)$")
var idRe = regexp.MustCompile("^([a-zA-Z_0-9-]+)$")
