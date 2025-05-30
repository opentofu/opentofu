// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package mcpprovider contains infrastructure for using Model Context Protocol
// server functionality as the basis for an OpenTofu provider.
//
// These providers act as MCP Clients mapping managed resource operations to
// MCP "tools" and data resource operations to MCP "resources", using a
// declarative language to project the MCP server features into OpenTofu
// provider features.
package mcpprovider
