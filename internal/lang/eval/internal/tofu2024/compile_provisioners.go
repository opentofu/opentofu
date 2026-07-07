// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	commShared "github.com/opentofu/opentofu/internal/communicator/shared"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// Based on NodeValidatableResource.validateProvisioner and NodeAbstractInstance.applyProvisioners

type compiledProvisioner func(ctx context.Context, localScope exprs.Scope, addr addrs.AbsResourceInstance) configgraph.Provisioner

func compileProvisioner(ctx context.Context,
	prov *configs.Provisioner,
	baseConn hcl.Body,
	provisioners evalglue.ProvisionersSchema,
) (compiledProvisioner, tfdiags.Diagnostics) {
	specConn := commShared.ConnectionBlockSupersetSchema.DecoderSpec()

	schema, err := provisioners.ProvisionerConfigSchema(ctx, prov.Type)
	if err != nil {
		// TODO This error probably won't be a great diagnostic
		return nil, tfdiags.New(err)
	}

	if schema == evalglue.StaticProvisionerSchema {
		// Not available in a static context
		return nil, nil
	}

	spec := schema.DecoderSpec()

	// If the provisioner block contains a connection block of its own then
	// it can override the base connection configuration, if any.
	var localConn hcl.Body
	if prov.Connection != nil {
		localConn = prov.Connection.Config
	}

	var connBody hcl.Body
	switch {
	case baseConn != nil && localConn != nil:
		// Our standard merging logic applies here, similar to what we do
		// with _override.tf configuration files: arguments from the
		// base connection block will be masked by any arguments of the
		// same name in the local connection block.
		connBody = configs.MergeBodies(baseConn, localConn)
	case baseConn != nil:
		connBody = baseConn
	case localConn != nil:
		connBody = localConn
	}

	return func(ctx context.Context, localScope exprs.Scope, addr addrs.AbsResourceInstance) configgraph.Provisioner {
		configFn := func(ctx context.Context, self cty.Value) (configgraph.ProvisionerConfig, tfdiags.Diagnostics) {
			var diags tfdiags.Diagnostics

			var selfIdent, selfName string
			if addr.Resource.Key == nil {
				// type.name syntax only allowed when no keys present
				// This is an intentional difference between the new engine and the original.
				selfIdent, selfName = addr.Resource.Resource.Type, addr.Resource.Resource.Name
			}

			provScope := selfScope(
				localScope,
				configgraph.WithoutResourceInstanceDependency(self, addr),
				selfIdent, selfName,
			)

			config, configDiags := exprs.NewClosure(exprs.EvalableHCLBody(prov.Config, spec), provScope).Value(ctx)
			diags = diags.Append(configDiags)

			// start with an empty connInfo
			connInfo := cty.NullVal(commShared.ConnectionBlockSupersetSchema.ImpliedType())

			if connBody != nil {
				var connInfoDiags tfdiags.Diagnostics
				connInfo, connInfoDiags = exprs.NewClosure(exprs.EvalableHCLBody(connBody, specConn), provScope).Value(ctx)
				diags = diags.Append(connInfoDiags)
			}
			return configgraph.ProvisionerConfig{
				Value:      config,
				Connection: connInfo,
			}, diags
		}

		dummyCfg, _ := configFn(ctx, cty.DynamicVal)

		return configgraph.Provisioner{
			Type:      prov.Type,
			When:      prov.When,
			OnFailure: prov.OnFailure,
			Config:    configFn,

			Dependencies: func(yield func(*configgraph.ResourceInstance) bool) {
				for ri := range configgraph.ContributingResourceInstances(dummyCfg.Value) {
					if !yield(ri) {
						return
					}
				}
				for ri := range configgraph.ContributingResourceInstances(dummyCfg.Connection) {
					if !yield(ri) {
						return
					}
				}
			},
		}
	}, nil
}
