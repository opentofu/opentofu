// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package execgraphproto contains just the protocol buffers models we use
// for marshaling and unmarshaling execution graphs.
//
// The logic for converting to and from the types defined in here is
// encapsulated in package execgraph. The specific serialization format for
// execution graphs is an implementation detail that may change arbitrarily
// between OpenTofu versions: it's not supported to take an execution graph
// marshaled by one OpenTofu version and then try to unmarshal it with a
// different OpenTofu version. Even the fact that there is an execution
// graph _at all_ is an implementation detail, with the public-facing model
// only representing a set of high-level actions against resource instances
// and output values.
package execgraphproto
