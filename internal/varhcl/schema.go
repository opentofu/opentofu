// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// This is a partial copy of hashicorp/hcl/gohcl/schema.go to allow access to internal variables.
// vardecode.go should be upstreamed instead.

package varhcl

import (
	"fmt"
	"reflect"
	"strings"
)

type fieldTags struct {
	Attributes map[string]int
	Blocks     map[string]int
	Labels     []labelField
	Remain     *int
	Body       *int
	Optional   map[string]bool
}

type labelField struct {
	FieldIndex int
	Name       string
}

func getFieldTags(ty reflect.Type) *fieldTags {
	ret := &fieldTags{
		Attributes: map[string]int{},
		Blocks:     map[string]int{},
		Optional:   map[string]bool{},
	}

	ct := ty.NumField()
	for i := 0; i < ct; i++ {
		field := ty.Field(i)
		tag := field.Tag.Get("hcl")
		if tag == "" {
			continue
		}

		comma := strings.Index(tag, ",")
		var name, kind string
		if comma != -1 {
			name = tag[:comma]
			kind = tag[comma+1:]
		} else {
			name = tag
			kind = "attr"
		}

		switch kind {
		case "attr":
			ret.Attributes[name] = i
		case "block":
			ret.Blocks[name] = i
		case "label":
			ret.Labels = append(ret.Labels, labelField{
				FieldIndex: i,
				Name:       name,
			})
		case "remain":
			if ret.Remain != nil {
				panic("only one 'remain' tag is permitted")
			}
			idx := i // copy, because this loop will continue assigning to i
			ret.Remain = &idx
		case "body":
			if ret.Body != nil {
				panic("only one 'body' tag is permitted")
			}
			idx := i // copy, because this loop will continue assigning to i
			ret.Body = &idx
		case "optional":
			ret.Attributes[name] = i
			ret.Optional[name] = true
		default:
			panic(fmt.Sprintf("invalid hcl field tag kind %q on %s %q", kind, field.Type.String(), field.Name))
		}
	}

	return ret
}
