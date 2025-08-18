// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package rpcproviders contains our main implementation of
// [providers.Interface] that handles most methods by making some sort of
// RPC request to a separate plugin process.
package rpcproviders
