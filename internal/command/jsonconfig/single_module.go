// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonconfig

import (
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/tofu"
)

// MarshalSingleModule is a variant of [Marshal] that describes only a single
// module, without any references to its child modules or associated provider
// schemas.
//
// This uses only a subset of the typical configuration representation, due to
// schema and child module information being unavailable:
//   - Module calls omit the "module" property that would normally describe the
//     content of the child module.
//   - Resource descriptions omit the "schema_version" property because no
//     schema-based information is included.
//   - Expression-related properties are omitted in all cases. Technically only
//     expressions passed to providers _need_ to be omitted, but for now we
//     just consistently omit all of them because that's an easier rule to
//     explain and avoids exposing what is and is not provider-based so that
//     we could potentially change those details in future.
func MarshalSingleModule(m *configs.Module) ([]byte, error) {
	// Our shared codepaths are built to work with a full config tree rather
	// than a single module, so we'll construct a synthetic [configs.Config]
	// that only has a root module and then downstream shared functions will
	// use the nil-ness of the schemas argument to handle the special
	// treatments required in single-module mode.
	cfg := &configs.Config{
		Module: m,
		// Everything else intentionally not populated because single module
		// mode should not attempt to access anything else.
	}
	return marshal(cfg, nil)
}

// inSingleModuleMode returns true if the given schema value indicates that
// we should be rendering in "single module" mode, meaning that we're producing
// a result for [MarshalSingleModule] rather than [Marshal].
//
// Currently the rule is only that a nil schemas represents single-module mode;
// this simple rule is factored out into this helper function only so we can
// centralize it underneath this doc comment explaining the special convention.
//
// (This rather odd design is a consequence of how this code evolved; we
// retrofitted the single-module mode later while using this strange treatment
// to minimize the risk to the existing working codepaths. Maybe we'll change
// the appoach to this in future; this is only an implementation detail
// within this package so we'll be able to those changes without affecting
// callers.)
func inSingleModuleMode[S schemaObject](schema S) bool {
	return schema == nil
}

// mapSchema is a helper that uses the given function to transform the given
// schema object only if it isn't nil, or immediately returns nil otherwise.
//
// This is part of our strategy to retrofit the single-module mode without
// a risky refactor of the already-working code, intended to be used in
// conjunction with [inSingleModuleMode] to smuggle the flag for whether we're
// in that mode through the nil-ness of the schema objects.
func mapSchema[In, Out schemaObject](schema In, f func(In) Out) Out {
	if schema == nil {
		return nil
	}
	return f(schema)
}

// schemaObject is a helper interface to allow [inSingleModuleMode] to be
// generic over the different nilable schema types used by different parts
// of the implementation in this package.
type schemaObject interface {
	*tofu.Schemas | *configschema.Block
}
