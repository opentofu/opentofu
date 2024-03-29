// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package gohcl

import (
	"fmt"
	"reflect"

	"github.com/zclconf/go-cty/cty"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/gocty"
)

// DecodeBody extracts the configuration within the given body into the given
// value. This value must be a non-nil pointer to either a struct or
// a map, where in the former case the configuration will be decoded using
// struct tags and in the latter case only attributes are allowed and their
// values are decoded into the map.
//
// The given EvalContext is used to resolve any variables or functions in
// expressions encountered while decoding. This may be nil to require only
// constant values, for simple applications that do not support variables or
// functions.
//
// The returned diagnostics should be inspected with its HasErrors method to
// determine if the populated value is valid and complete. If error diagnostics
// are returned then the given value may have been partially-populated but
// may still be accessed by a careful caller for static analysis and editor
// integration use-cases.
func DecodeBody(body hcl.Body, ctx *hcl.EvalContext, val interface{}) hcl.Diagnostics {
	rv := reflect.ValueOf(val)
	if rv.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("target value must be a pointer, not %s", rv.Type().String()))
	}

	return decodeBodyToValue(body, ctx, rv.Elem())
}

func decodeBodyToValue(body hcl.Body, ctx *hcl.EvalContext, val reflect.Value) hcl.Diagnostics {
	et := val.Type()
	switch et.Kind() {
	case reflect.Struct:
		return decodeBodyToStruct(body, ctx, val)
	case reflect.Map:
		return decodeBodyToMap(body, ctx, val)
	default:
		panic(fmt.Sprintf("target value must be pointer to struct or map, not %s", et.String()))
	}
}

func decodeBodyToStruct(body hcl.Body, ctx *hcl.EvalContext, val reflect.Value) hcl.Diagnostics {
	schema, partial := ImpliedBodySchema(val.Interface())

	var content *hcl.BodyContent
	var leftovers hcl.Body
	var diags hcl.Diagnostics
	if partial {
		content, leftovers, diags = body.PartialContent(schema)
	} else {
		content, diags = body.Content(schema)
	}
	if content == nil {
		return diags
	}

	tags := getFieldTags(val.Type())

	if tags.Body != nil {
		fieldIdx := *tags.Body
		field := val.Type().Field(fieldIdx)
		fieldV := val.Field(fieldIdx)
		switch {
		case bodyType.AssignableTo(field.Type):
			fieldV.Set(reflect.ValueOf(body))

		default:
			diags = append(diags, decodeBodyToValue(body, ctx, fieldV)...)
		}
	}

	if tags.Remain != nil {
		fieldIdx := *tags.Remain
		field := val.Type().Field(fieldIdx)
		fieldV := val.Field(fieldIdx)
		switch {
		case bodyType.AssignableTo(field.Type):
			fieldV.Set(reflect.ValueOf(leftovers))
		case attrsType.AssignableTo(field.Type):
			attrs, attrsDiags := leftovers.JustAttributes()
			if len(attrsDiags) > 0 {
				diags = append(diags, attrsDiags...)
			}
			fieldV.Set(reflect.ValueOf(attrs))
		default:
			diags = append(diags, decodeBodyToValue(leftovers, ctx, fieldV)...)
		}
	}

	for name, fieldIdx := range tags.Attributes {
		attr := content.Attributes[name]
		field := val.Type().Field(fieldIdx)
		fieldV := val.Field(fieldIdx)

		if attr == nil {
			if !exprType.AssignableTo(field.Type) {
				continue
			}

			// As a special case, if the target is of type hcl.Expression then
			// we'll assign an actual expression that evalues to a cty null,
			// so the caller can deal with it within the cty realm rather
			// than within the Go realm.
			synthExpr := hcl.StaticExpr(cty.NullVal(cty.DynamicPseudoType), body.MissingItemRange())
			fieldV.Set(reflect.ValueOf(synthExpr))
			continue
		}

		switch {
		case attrType.AssignableTo(field.Type):
			fieldV.Set(reflect.ValueOf(attr))
		case exprType.AssignableTo(field.Type):
			fieldV.Set(reflect.ValueOf(attr.Expr))
		case field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct:
			// TODO might want to check for nil here
			rn := reflect.New(field.Type.Elem())
			fieldV.Set(rn)
			diags = append(diags, DecodeExpression(
				attr.Expr, ctx, fieldV.Interface(),
			)...)
		default:
			diags = append(diags, DecodeExpression(
				attr.Expr, ctx, fieldV.Addr().Interface(),
			)...)
		}
	}

	blocksByType := content.Blocks.ByType()

	for typeName, fieldIdx := range tags.Blocks {
		blocks := blocksByType[typeName]
		field := val.Type().Field(fieldIdx)

		ty := field.Type
		isSlice := false
		isPtr := false
		if ty.Kind() == reflect.Slice {
			isSlice = true
			ty = ty.Elem()
		}
		if ty.Kind() == reflect.Ptr {
			isPtr = true
			ty = ty.Elem()
		}

		if len(blocks) > 1 && !isSlice {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  fmt.Sprintf("Duplicate %s block", typeName),
				Detail: fmt.Sprintf(
					"Only one %s block is allowed. Another was defined at %s.",
					typeName, blocks[0].DefRange.String(),
				),
				Subject: &blocks[1].DefRange,
			})
			continue
		}

		if len(blocks) == 0 {
			if isSlice || isPtr {
				if val.Field(fieldIdx).IsNil() {
					val.Field(fieldIdx).Set(reflect.Zero(field.Type))
				}
			} else {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  fmt.Sprintf("Missing %s block", typeName),
					Detail:   fmt.Sprintf("A %s block is required.", typeName),
					Subject:  body.MissingItemRange().Ptr(),
				})
			}
			continue
		}

		switch {

		case isSlice:
			elemType := ty
			if isPtr {
				elemType = reflect.PtrTo(ty)
			}
			sli := val.Field(fieldIdx)
			if sli.IsNil() {
				sli = reflect.MakeSlice(reflect.SliceOf(elemType), len(blocks), len(blocks))
			}

			for i, block := range blocks {
				if isPtr {
					if i >= sli.Len() {
						sli = reflect.Append(sli, reflect.New(ty))
					}
					v := sli.Index(i)
					if v.IsNil() {
						v = reflect.New(ty)
					}
					diags = append(diags, decodeBlockToValue(block, ctx, v.Elem())...)
					sli.Index(i).Set(v)
				} else {
					if i >= sli.Len() {
						sli = reflect.Append(sli, reflect.Indirect(reflect.New(ty)))
					}
					diags = append(diags, decodeBlockToValue(block, ctx, sli.Index(i))...)
				}
			}

			if sli.Len() > len(blocks) {
				sli.SetLen(len(blocks))
			}

			val.Field(fieldIdx).Set(sli)

		default:
			block := blocks[0]
			if isPtr {
				v := val.Field(fieldIdx)
				if v.IsNil() {
					v = reflect.New(ty)
				}
				diags = append(diags, decodeBlockToValue(block, ctx, v.Elem())...)
				val.Field(fieldIdx).Set(v)
			} else {
				diags = append(diags, decodeBlockToValue(block, ctx, val.Field(fieldIdx))...)
			}

		}

	}

	return diags
}

func decodeBodyToMap(body hcl.Body, ctx *hcl.EvalContext, v reflect.Value) hcl.Diagnostics {
	attrs, diags := body.JustAttributes()
	if attrs == nil {
		return diags
	}

	mv := reflect.MakeMap(v.Type())

	for k, attr := range attrs {
		switch {
		case attrType.AssignableTo(v.Type().Elem()):
			mv.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(attr))
		case exprType.AssignableTo(v.Type().Elem()):
			mv.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(attr.Expr))
		default:
			ev := reflect.New(v.Type().Elem())
			diags = append(diags, DecodeExpression(attr.Expr, ctx, ev.Interface())...)
			mv.SetMapIndex(reflect.ValueOf(k), ev.Elem())
		}
	}

	v.Set(mv)

	return diags
}

func decodeBlockToValue(block *hcl.Block, ctx *hcl.EvalContext, v reflect.Value) hcl.Diagnostics {
	diags := decodeBodyToValue(block.Body, ctx, v)

	if len(block.Labels) > 0 {
		blockTags := getFieldTags(v.Type())
		for li, lv := range block.Labels {
			lfieldIdx := blockTags.Labels[li].FieldIndex
			v.Field(lfieldIdx).Set(reflect.ValueOf(lv))
		}
	}

	return diags
}

// DecodeExpression extracts the value of the given expression into the given
// value. This value must be something that gocty is able to decode into,
// since the final decoding is delegated to that package.  If a reference to
// a struct is provided which contains gohcl tags, it will be decoded using
// the attr and optional tags.
//
// The given EvalContext is used to resolve any variables or functions in
// expressions encountered while decoding. This may be nil to require only
// constant values, for simple applications that do not support variables or
// functions.
//
// The returned diagnostics should be inspected with its HasErrors method to
// determine if the populated value is valid and complete. If error diagnostics
// are returned then the given value may have been partially-populated but
// may still be accessed by a careful caller for static analysis and editor
// integration use-cases.
func DecodeExpression(expr hcl.Expression, ctx *hcl.EvalContext, val interface{}) hcl.Diagnostics {
	srcVal, diags := expr.Value(ctx)
	if diags.HasErrors() {
		return diags
	}

	return append(diags, DecodeValue(srcVal, expr.StartRange(), expr.Range(), val)...)
}

// DecodeValue extracts the given value into the provided target.
// This value must be something that gocty is able to decode into,
// since the final decoding is delegated to that package.  If a reference to
// a struct is provided which contains gohcl tags, it will be decoded using
// the attr and optional tags.
//
// The returned diagnostics should be inspected with its HasErrors method to
// determine if the populated value is valid and complete. If error diagnostics
// are returned then the given value may have been partially-populated but
// may still be accessed by a careful caller for static analysis and editor
// integration use-cases.
func DecodeValue(srcVal cty.Value, subject hcl.Range, context hcl.Range, val interface{}) hcl.Diagnostics {
	rv := reflect.ValueOf(val)
	if rv.Type().Kind() == reflect.Ptr && rv.Type().Elem().Kind() == reflect.Struct && hasFieldTags(rv.Elem().Type()) {
		attrs := make(hcl.Attributes)
		for k, v := range srcVal.AsValueMap() {
			attrs[k] = &hcl.Attribute{
				Name:  k,
				Expr:  hcl.StaticExpr(v, context),
				Range: subject,
			}

		}
		return decodeBodyToStruct(synthBody{
			attrs:   attrs,
			subject: subject,
			context: context,
		}, nil, rv.Elem())

	}

	convTy, err := gocty.ImpliedType(val)
	if err != nil {
		panic(fmt.Sprintf("unsuitable DecodeExpression target: %s", err))
	}

	var diags hcl.Diagnostics

	srcVal, err = convert.Convert(srcVal, convTy)
	if err != nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unsuitable value type",
			Detail:   fmt.Sprintf("Unsuitable value: %s", err.Error()),
			Subject:  subject.Ptr(),
			Context:  context.Ptr(),
		})
		return diags
	}

	err = gocty.FromCtyValue(srcVal, val)
	if err != nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unsuitable value type",
			Detail:   fmt.Sprintf("Unsuitable value: %s", err.Error()),
			Subject:  subject.Ptr(),
			Context:  context.Ptr(),
		})
	}

	return diags
}

type synthBody struct {
	attrs   hcl.Attributes
	subject hcl.Range
	context hcl.Range
}

func (s synthBody) Content(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Diagnostics) {
	body, partial, diags := s.PartialContent(schema)

	attrs, _ := partial.JustAttributes()
	for name := range attrs {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unsupported argument",
			Detail:   fmt.Sprintf("An argument named %q is not expected here.", name),
			Subject:  s.subject.Ptr(),
			Context:  s.context.Ptr(),
		})
	}

	return body, diags
}

func (s synthBody) PartialContent(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Body, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	for _, block := range schema.Blocks {
		panic("hcl block tags are not allowed in attribute structs: " + block.Type)
	}

	attrs := make(hcl.Attributes)
	remainder := make(hcl.Attributes)

	for _, attr := range schema.Attributes {
		v, ok := s.attrs[attr.Name]
		if !ok {
			if attr.Required {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Missing required argument",
					Detail:   fmt.Sprintf("The argument %q is required, but no definition was found.", attr.Name),
					Subject:  s.subject.Ptr(),
					Context:  s.context.Ptr(),
				})
			}
			continue
		}

		attrs[attr.Name] = v
	}

	for k, v := range s.attrs {
		if _, ok := attrs[k]; !ok {
			remainder[k] = v
		}
	}

	return &hcl.BodyContent{
		Attributes:       attrs,
		MissingItemRange: s.context,
	}, synthBody{attrs: remainder}, diags
}

func (s synthBody) JustAttributes() (hcl.Attributes, hcl.Diagnostics) {
	return s.attrs, nil
}
func (s synthBody) MissingItemRange() hcl.Range {
	return s.context
}
