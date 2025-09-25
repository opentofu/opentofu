// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package containersconfig contains types used for describing (or automatically
// discovering) container-execution-related settings.
//
// The rest of OpenTofu should make use of this package only indirectly through
// package cliconfig, which is responsible for making the policy decisions.
package containersconfig
