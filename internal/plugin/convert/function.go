package convert

import (
	"encoding/json"
	"fmt"

	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfplugin5"
	"github.com/zclconf/go-cty/cty"
)

func ProtoToCtyType(in []byte) cty.Type {
	var out cty.Type
	if err := json.Unmarshal(in, &out); err != nil {
		panic(err)
	}
	return out
}
func CtyTypeToProto(in cty.Type) []byte {
	out, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}
	return out
}

func ProtoToTextFormatting(proto tfplugin5.StringKind) providers.TextFormatting {
	switch proto {
	case tfplugin5.StringKind_PLAIN:
		return providers.TextFormattingPlain
	case tfplugin5.StringKind_MARKDOWN:
		return providers.TextFormattingMarkdown
	default:
		panic(fmt.Sprintf("Invalid text tfplugin5.StringKind %v", proto))
	}
}

func TextFormattingToProto(spec providers.TextFormatting) tfplugin5.StringKind {
	switch spec {
	case "":
		fallthrough
	case providers.TextFormattingPlain:
		return tfplugin5.StringKind_PLAIN
	case providers.TextFormattingMarkdown:
		return tfplugin5.StringKind_MARKDOWN
	default:
		panic(fmt.Sprintf("Invalid TextFormatting %v", spec))
	}
}

func ProtoToFunctionParameterSpec(proto *tfplugin5.Function_Parameter) providers.FunctionParameterSpec {
	return providers.FunctionParameterSpec{
		Name:               proto.Name,
		Type:               ProtoToCtyType(proto.Type),
		AllowNullValue:     proto.AllowNullValue,
		AllowUnknownValues: proto.AllowUnknownValues,
		Description:        proto.Description,
		DescriptionFormat:  ProtoToTextFormatting(proto.DescriptionKind),
	}
}

func FunctionParameterSpecToProto(spec providers.FunctionParameterSpec) *tfplugin5.Function_Parameter {
	return &tfplugin5.Function_Parameter{
		Name:               spec.Name,
		Type:               CtyTypeToProto(spec.Type),
		AllowNullValue:     spec.AllowNullValue,
		AllowUnknownValues: spec.AllowUnknownValues,
		Description:        spec.Description,
		DescriptionKind:    TextFormattingToProto(spec.DescriptionFormat),
	}
}

func ProtoToFunctionSpec(proto *tfplugin5.Function) providers.FunctionSpec {
	params := make([]providers.FunctionParameterSpec, len(proto.Parameters))
	for i, param := range proto.Parameters {
		params[i] = ProtoToFunctionParameterSpec(param)
	}

	var varParam *providers.FunctionParameterSpec
	if proto.VariadicParameter != nil {
		param := ProtoToFunctionParameterSpec(proto.VariadicParameter)
		varParam = &param
	}

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

func FunctionSpecToProto(spec providers.FunctionSpec) *tfplugin5.Function {
	params := make([]*tfplugin5.Function_Parameter, len(spec.Parameters))
	for i, param := range spec.Parameters {
		params[i] = FunctionParameterSpecToProto(param)
	}

	var varParam *tfplugin5.Function_Parameter
	if spec.VariadicParameter != nil {
		varParam = FunctionParameterSpecToProto(*spec.VariadicParameter)
	}

	return &tfplugin5.Function{
		Parameters:         params,
		VariadicParameter:  varParam,
		Return:             &tfplugin5.Function_Return{Type: CtyTypeToProto(spec.Return)},
		Summary:            spec.Summary,
		Description:        spec.Description,
		DescriptionKind:    TextFormattingToProto(spec.DescriptionFormat),
		DeprecationMessage: spec.DeprecationMessage,
	}
}
