// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package exec contains the models and main interface used for apply phase
// execution.
//
// The types and functions in this package are typically used through the
// sibling package [execgraph] to model the data flow and dependencies between
// multiple operations, but the individual types, interfaces, and functions
// are exposed here to make it possible to write tests for individual parts
// of the apply engine without having to always build an execution graph.
//
// These "vocabulary types" are separated into their own package mainly to
// minimize the risk of dependency cycles as other components make use of them.
// This package does not import any other package that is considered to be a
// component of the apply engine, but it can import packages that other parts
// of the apply engine are also expected to depend on, such as the packages
// which model state objects and provider clients.
package exec
