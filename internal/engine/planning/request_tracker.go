// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"iter"

	"github.com/apparentlymart/go-workgraph/workgraph"

	"github.com/opentofu/opentofu/internal/lang/grapheval"
)

func contextWithRequestTracker(parent context.Context, glue *planGlue) context.Context {
	return grapheval.ContextWithRequestTracker(parent, planRequestTracker{
		glue: glue,
	})
}

// planRequestTracker is our implementation of [grapheval.RequestTracker] for
// use when doing our planning work.
//
// An instance of this should be associated with the context passed to
// everything we do during planning so that we can use it to produce helpful
// error messages if promise-related errors occur during execution.
type planRequestTracker struct {
	glue *planGlue
}

var _ grapheval.RequestTracker = (*planRequestTracker)(nil)

// ActiveRequests implements [grapheval.RequestTracker].
func (t planRequestTracker) ActiveRequests() iter.Seq2[workgraph.RequestID, grapheval.RequestInfo] {
	return grapheval.PushActiveRequests(t.glue.AnnounceAllGraphevalRequests)
}
