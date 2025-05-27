// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

//go:generate go run golang.org/x/tools/cmd/stringer -type=ResourceMode -output=resource_mode_string.go resource_mode.go

// ResourceMode is deprecated, use addrs.ResourceMode instead.
// It has been preserved for backwards compatibility.
type ResourceMode int

const (
	ManagedResourceMode ResourceMode = iota
	DataResourceMode
)
