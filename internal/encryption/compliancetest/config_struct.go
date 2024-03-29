// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package compliancetest

import (
	"reflect"
	"strings"
	"testing"
)

func ConfigStruct[TConfig any](t *testing.T, configStruct any) {
	Log(t, "Testing config struct compliance...")
	if configStruct == nil {
		Fail(t, "The ConfigStruct() method on the descriptor returns a nil configuration. Please implement this function correctly.")
	} else {
		Log(t, "The ConfigStruct() method returned a non-nil value.")
	}

	configStructPtrType := reflect.TypeOf(configStruct)
	if configStructPtrType.Kind() != reflect.Ptr {
		Fail(t, "The ConfigStruct() method returns a %T, but it should return a pointer to a struct.", configStruct)
	} else {
		Log(t, "The ConfigStruct() method returned a pointer.")
	}
	configStructType := configStructPtrType.Elem()
	if configStructType.Kind() != reflect.Struct {
		Fail(t, "The ConfigStruct() method returns a pointer to %s, but it should return a pointer to a struct.", configStructType.Elem().Name())
	} else {
		Log(t, "The ConfigStruct() method returned a pointer to a struct.")
	}

	typedConfigStruct, ok := configStruct.(TConfig)
	if !ok {
		Fail(t, "The ConfigStruct() method returns a %T instead of a %T", configStruct, typedConfigStruct)
	} else {
		Log(t, "The ConfigStruct() method correctly returns a %T", typedConfigStruct)
	}

	hclTagFound := false
	for i := 0; i < configStructType.NumField(); i++ {
		field := configStructType.Field(i)
		hclTag, ok := field.Tag.Lookup("hcl")
		if !ok {
			continue
		}
		hclTagFound = true
		if hclTag == "" {
			Fail(
				t,
				"The field '%s' on the config structure %s has an empty HCL tag. Please remove the hcl tag or add a value that matches %s.",
				field.Name,
				configStructType.Name(),
				hclTagRe,
			)
		} else {
			Log(t, "Found a non-empty hcl tag on field '%s' of %s.", field.Name, configStructType.Name())
		}
		hclTagParts := strings.Split(hclTag, ",")
		if !hclTagRe.MatchString(hclTagParts[0]) {
			Fail(
				t,
				"The field '%s' on the config structure %s has an invalid hcl tag: %s. Please add a value that matches %s.",
				field.Name,
				configStructType.Name(),
				hclTag,
				hclTagRe,
			)
		} else {
			Log(t, "Found hcl tag on field '%s' of %s matches the name requirements.", field.Name, configStructType.Name())
		}
	}
	if !hclTagFound {
		Fail(
			t,
			"The configuration struct %s does not contain any fields with hcl tags, which means users will not be able to configure this key provider. Please provide at least one field with an hcl tag.",
			configStructType.Name(),
		)
	} else {
		Log(t, "Found at least one field with a hcl tag.")
	}

}
