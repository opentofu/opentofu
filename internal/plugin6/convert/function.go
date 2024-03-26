package convert

import (
	"encoding/json"
	"fmt"

	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfplugin6"
	"github.com/zclconf/go-cty/cty"
)

func ProtoToCtyType(in []byte) cty.Type {
	var out cty.Type
	if err := json.Unmarshal(in, &out); err != nil {
		panic(err)
	}
	return out
}

func ProtoToTextFormatting(proto tfplugin6.StringKind) providers.TextFormatting {
	switch proto {
	case tfplugin6.StringKind_PLAIN:
		return providers.TextFormattingPlain
	case tfplugin6.StringKind_MARKDOWN:
		return providers.TextFormattingMarkdown
	default:
		panic(fmt.Sprintf("Invalid text tfplugin6.StringKind %v", proto))
	}
}

func ProtoToFunctionParameterSpec(proto *tfplugin6.Function_Parameter) providers.FunctionParameterSpec {
	return providers.FunctionParameterSpec{
		Name:               proto.Name,
		Type:               ProtoToCtyType(proto.Type),
		AllowNullValue:     proto.AllowNullValue,
		AllowUnknownValues: proto.AllowUnknownValues,
		Description:        proto.Description,
		DescriptionFormat:  ProtoToTextFormatting(proto.DescriptionKind),
	}
}

func ProtoToFunctionSpec(proto *tfplugin6.Function) providers.FunctionSpec {
	params := make([]providers.FunctionParameterSpec, len(proto.Parameters))
	for i, param := range proto.Parameters {
		params[i] = ProtoToFunctionParameterSpec(param)
	}

	var varParam *providers.FunctionParameterSpec
	if proto.VariadicParameter != nil {
		param := ProtoToFunctionParameterSpec(proto.VariadicParameter)
		varParam = &param
	}

	// *FunctionParameterSpec
	return providers.FunctionSpec{
		Parameters:         params,
		VariadicParameter:  varParam,
		Return:             ProtoToCtyType(proto.Return.Type),
		Summary:            proto.Summary,
		Description:        proto.Description,
		DescriptionFormat:  ProtoToTextFormatting(proto.DescriptionKind),
		DeprecationMessage: proto.DeprecationMessage,
	}
}
