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
func CtyTypeToProto(in cty.Type) []byte {
	out, err := json.Marshal(in)
	if err != nil {
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

func TextFormattingToProto(spec providers.TextFormatting) tfplugin6.StringKind {
	switch spec {
	case "":
		fallthrough
	case providers.TextFormattingPlain:
		return tfplugin6.StringKind_PLAIN
	case providers.TextFormattingMarkdown:
		return tfplugin6.StringKind_MARKDOWN
	default:
		panic(fmt.Sprintf("Invalid TextFormatting %v", spec))
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

func FunctionParameterSpecToProto(spec providers.FunctionParameterSpec) *tfplugin6.Function_Parameter {
	return &tfplugin6.Function_Parameter{
		Name:               spec.Name,
		Type:               CtyTypeToProto(spec.Type),
		AllowNullValue:     spec.AllowNullValue,
		AllowUnknownValues: spec.AllowUnknownValues,
		Description:        spec.Description,
		DescriptionKind:    TextFormattingToProto(spec.DescriptionFormat),
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

func FunctionSpecToProto(spec providers.FunctionSpec) *tfplugin6.Function {
	params := make([]*tfplugin6.Function_Parameter, len(spec.Parameters))
	for i, param := range spec.Parameters {
		params[i] = FunctionParameterSpecToProto(param)
	}

	var varParam *tfplugin6.Function_Parameter
	if spec.VariadicParameter != nil {
		varParam = FunctionParameterSpecToProto(*spec.VariadicParameter)
	}

	return &tfplugin6.Function{
		Parameters:         params,
		VariadicParameter:  varParam,
		Return:             &tfplugin6.Function_Return{Type: CtyTypeToProto(spec.Return)},
		Summary:            spec.Summary,
		Description:        spec.Description,
		DescriptionKind:    TextFormattingToProto(spec.DescriptionFormat),
		DeprecationMessage: spec.DeprecationMessage,
	}
}
