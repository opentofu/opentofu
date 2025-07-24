// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package rpcproviders

import (
	"context"
	"errors"
	"fmt"

	"github.com/apparentlymart/opentofu-providers/tofuprovider/providerops"
	"github.com/apparentlymart/opentofu-providers/tofuprovider/providerschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// CallFunction implements providers.Interface.
func (r rpcProvider) CallFunction(ctx context.Context, req providers.CallFunctionRequest) providers.CallFunctionResponse {
	var resp providers.CallFunctionResponse

	spec, diags := r.schema.GetFunction(ctx, req.Name)
	if diags.HasErrors() {
		// Annoyingly this particular operation does not return diagnostics
		// and instead can return only zero or one errors, so we'll
		// just smuggle all of the diagnostics through that error here.
		resp.Error = diags.Err()
		return resp
	}
	if spec == nil {
		resp.Error = fmt.Errorf("provider does not have function named %q", req.Name)
		return resp
	}

	realReq := &providerops.CallFunctionRequest{
		FunctionName: req.Name,
		Arguments:    make([]providerschema.DynamicValueIn, len(req.Arguments)),
	}
	for i, arg := range req.Arguments {
		var param providers.FunctionParameterSpec
		if i < len(spec.Parameters) {
			param = spec.Parameters[i]
		} else {
			if spec.VariadicParameter == nil {
				resp.Error = function.NewArgErrorf(i, "too many parameters")
				return resp
			}
		}

		if !spec.VariadicParameter.AllowUnknownValues {
			if !arg.IsWhollyKnown() {
				resp.Result = cty.UnknownVal(spec.Return)
				return resp
			}
		}
		if !spec.VariadicParameter.AllowNullValue {
			if arg.IsNull() {
				resp.Error = function.NewArgErrorf(i, "must not be null")
				return resp
			}
		}

		realReq.Arguments[i] = providerschema.NewDynamicValue(arg, param.Type)
	}

	realResp, err := r.client.CallFunction(ctx, realReq)
	if err != nil {
		resp.Error = fmt.Errorf("provider call failed: %w", err)
		return resp
	}
	if funcErr := realResp.Error(); err != nil {
		err := errors.New(funcErr.Text())
		if argIdx, ok := funcErr.ArgumentIndex(); ok {
			resp.Error = function.NewArgError(argIdx, err)
		} else {
			resp.Error = err
		}
		return resp
	}

	retVal, err := realResp.Result().AsCtyValue(spec.Return)
	if err != nil {
		resp.Error = fmt.Errorf("provider returned invalid result: %w", err)
		return resp
	}
	resp.Result = retVal
	return resp
}
