// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package uritemplates implements the URI Templates language described in [RFC 6570].
//
// This package is used to support the use of URI templates as part of some service definitions
// in OpenTofu's network service discovery protocol, which currently supports only
// Level 1 templates to reduce complexity, because OpenTofu services tend to follow a
// prescriptive URL scheme that doesn't require advanced URI template features like
// constructing a query string.
//
// If those needs increase in future then the scope of this package might increase to
// follow, or we might adopt an external dependency implementing this specification instead.
package uritemplates
