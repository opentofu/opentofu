// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

// graphNodeRetainedByPruneUnusedNodesTransformer is a marker interface.
// A Node implementing this interface means it should not be pruned during pruneUnusedNodesTransformer.
// This interface is primarily used by nodes that expand instances but not restricted to them.
// (Two node types implementing this interface without implementing GraphNodeDynamicExpandable are
// nodeExpandModule and nodeExpandApplyableResource)
//
// Keep in mind that due to how this is used by pruneUnusedNodesTransformer, the node must have `UpEdges`
// (Graph vertices coming into it), one of which must be either of the following:
// GraphNodeProvider, GraphNodeResourceInstance, GraphNodeDynamicExpandable or graphNodeRetainedByPruneUnusedNodesTransformer itself
// to be retained during pruning.
// nodeExpandCheck is a good example of a node supposed to be implementing this interface, but due to the aforementioned
// limitation in the current implementation is not.
// More details in this issue https://github.com/opentofu/opentofu/issues/2808
type graphNodeRetainedByPruneUnusedNodesTransformer interface {
	retainDuringUnusedPruning()
}
