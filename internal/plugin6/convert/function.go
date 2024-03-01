package convert

import (
	"encoding/json"

	proto "github.com/opentofu/opentofu/internal/tfplugin6"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func ProtoToFunctionParameter(param *proto.Function_Parameter) function.Parameter {
	var paramType cty.Type
	if err := json.Unmarshal(param.Type, &paramType); err != nil {
		panic(err)
	}

	return function.Parameter{
		Name:         param.Name,
		Description:  param.Description,
		Type:         paramType,
		AllowNull:    param.AllowNullValue,
		AllowUnknown: param.AllowUnknownValues,
		// DescriptionKind ignored
		// AllowDynamicType unset
		// AllowMarked unset
	}
}

func ProtoToFunctionSpec(fn proto.Function) (out function.Spec) {
	spec := function.Spec{}

	spec.Description = fn.Description

	// Convert proto params to spec params
	spec.Params = make([]function.Parameter, len(fn.Parameters))
	for i, param := range fn.Parameters {
		spec.Params[i] = ProtoToFunctionParameter(param)
	}

	if fn.VariadicParameter != nil {
		param := ProtoToFunctionParameter(fn.VariadicParameter)
		spec.VarParam = &param
	}

	var retType cty.Type
	if err := json.Unmarshal(fn.Return.Type, &retType); err != nil {
		panic(err)
	}
	spec.Type = func(args []cty.Value) (cty.Type, error) {
		return retType, nil
	}

	// DescriptionKind ignored
	// Summary ignored
	// DeprecationMessage ignored

	// Impl must be set later

	return spec

}
