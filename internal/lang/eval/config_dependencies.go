// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type FindDependenciesGlue interface {
}

// FindDependencies evaluates the called configuration in a special limited mode
// that aims only to learn which external module packages and providers this
// instance of the configuration depends on.
//
// This uses a very limited evaluation context where no providers are available
// at all (because our goal is to find out which we need!) and where the
// caller must be prepared to fetch remote module packages and load modules
// from them during the process, through the provided [FindDependenciesGlue]
// implementation.
func FindDependencies(
	ctx context.Context,
	rootModuleSource *addrs.ModuleSource,
	inputValues map[addrs.InputVariable]exprs.Valuer,
	glue FindDependenciesGlue,
) (*DependenciesRequired, tfdiags.Diagnostics) {
	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)
	_ = ctx // just so we can keep the above as a reminder of the need to have a grapheval worker in future work

	panic("unimplemented")
}

// DependenciesRequired is the result of [FindDependencies], describing
// the dependencies that would need to be available to successfully work with
// a configuration instance built from the given root module and input values.
type DependenciesRequired struct {
}
