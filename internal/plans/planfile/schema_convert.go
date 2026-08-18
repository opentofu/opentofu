// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planfile

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plans/internal/planproto"
	"github.com/opentofu/opentofu/internal/providers"
)

func schemaToProto(s providers.Schema) *planproto.Schema {
	return &planproto.Schema{
		Version: s.Version,
		Block:   configSchemaBlockToProto(s.Block),
	}
}

func schemaFromProto(s *planproto.Schema) providers.Schema {
	if s == nil {
		return providers.Schema{}
	}
	return providers.Schema{
		Version: s.Version,
		Block:   configSchemaBlockFromProto(s.Block),
	}
}

func resourceIdentitySchemaToProto(version int64, body *configschema.Object) *planproto.ResourceIdentitySchema {
	if body == nil {
		return nil
	}
	return &planproto.ResourceIdentitySchema{
		Version: version,
		Body:    configSchemaObjectToProto(body),
	}
}

// resourceSchemaToProto packages a resource type's (or data source's)
// configuration schema together with its resource identity schema, if any.
// identityVersion/identityBody should both be zero/nil for data sources,
// which have no resource identity concept.
func resourceSchemaToProto(schema providers.Schema) *planproto.ResourceSchema {
	return &planproto.ResourceSchema{
		Schema:         schemaToProto(schema),
		IdentitySchema: resourceIdentitySchemaToProto(schema.IdentitySchemaVersion, schema.IdentitySchema),
	}
}

func resourceSchemaFromProto(rs *planproto.ResourceSchema) providers.Schema {
	if rs == nil {
		return providers.Schema{}
	}
	s := schemaFromProto(rs.Schema)
	if rs.IdentitySchema != nil {
		s.IdentitySchemaVersion = rs.IdentitySchema.Version
		s.IdentitySchema = configSchemaObjectFromProto(rs.IdentitySchema.Body)
	}
	return s
}

func configSchemaBlockToProto(b *configschema.Block) *planproto.SchemaBlock {
	if b == nil {
		return nil
	}
	block := &planproto.SchemaBlock{}

	for _, name := range slices.Sorted(maps.Keys(b.Attributes)) {
		block.Attributes = append(block.Attributes, configSchemaAttributeToProto(name, b.Attributes[name]))
	}
	for _, name := range slices.Sorted(maps.Keys(b.BlockTypes)) {
		block.BlockTypes = append(block.BlockTypes, configSchemaNestedBlockToProto(name, b.BlockTypes[name]))
	}

	return block
}

func configSchemaBlockFromProto(b *planproto.SchemaBlock) *configschema.Block {
	block := &configschema.Block{
		Attributes: make(map[string]*configschema.Attribute),
		BlockTypes: make(map[string]*configschema.NestedBlock),
	}
	if b == nil {
		return block
	}

	for _, a := range b.Attributes {
		block.Attributes[a.Name] = configSchemaAttributeFromProto(a)
	}
	for _, nb := range b.BlockTypes {
		block.BlockTypes[nb.TypeName] = configSchemaNestedBlockFromProto(nb)
	}

	return block
}

func configSchemaAttributeToProto(name string, a *configschema.Attribute) *planproto.SchemaAttribute {
	attr := &planproto.SchemaAttribute{
		Name:      name,
		Required:  a.Required,
		Optional:  a.Optional,
		Computed:  a.Computed,
		Sensitive: a.Sensitive,
		WriteOnly: a.WriteOnly,
	}

	if a.NestedType != nil {
		attr.NestedType = configSchemaObjectToProto(a.NestedType)
	} else if a.Type != cty.NilType {
		ty, err := json.Marshal(a.Type)
		if err != nil {
			// cty.Type always marshals successfully; a failure here would
			// indicate a bug (an invalid type was constructed somewhere).
			panic(fmt.Errorf("failed to marshal attribute type: %w", err))
		}
		attr.Type = ty
	}

	return attr
}

func configSchemaAttributeFromProto(a *planproto.SchemaAttribute) *configschema.Attribute {
	attr := &configschema.Attribute{
		Required:  a.Required,
		Optional:  a.Optional,
		Computed:  a.Computed,
		Sensitive: a.Sensitive,
		WriteOnly: a.WriteOnly,
	}

	if a.NestedType != nil {
		attr.NestedType = configSchemaObjectFromProto(a.NestedType)
	} else if len(a.Type) > 0 {
		if err := json.Unmarshal(a.Type, &attr.Type); err != nil {
			panic(fmt.Errorf("failed to unmarshal attribute type: %w", err))
		}
	}

	return attr
}

func configSchemaObjectToProto(o *configschema.Object) *planproto.SchemaObject {
	if o == nil {
		return nil
	}
	obj := &planproto.SchemaObject{
		Nesting: nestingModeToProto(o.Nesting),
	}
	for _, name := range slices.Sorted(maps.Keys(o.Attributes)) {
		obj.Attributes = append(obj.Attributes, configSchemaAttributeToProto(name, o.Attributes[name]))
	}
	return obj
}

func configSchemaObjectFromProto(o *planproto.SchemaObject) *configschema.Object {
	obj := &configschema.Object{
		Attributes: make(map[string]*configschema.Attribute),
	}
	if o == nil {
		return obj
	}
	obj.Nesting = nestingModeFromProto(o.Nesting)
	for _, a := range o.Attributes {
		obj.Attributes[a.Name] = configSchemaAttributeFromProto(a)
	}
	return obj
}

func configSchemaNestedBlockToProto(name string, nb *configschema.NestedBlock) *planproto.SchemaNestedBlock {
	return &planproto.SchemaNestedBlock{
		TypeName: name,
		Block:    configSchemaBlockToProto(&nb.Block),
		Nesting:  nestingModeToProto(nb.Nesting),
		MinItems: int64(nb.MinItems),
		MaxItems: int64(nb.MaxItems),
	}
}

func configSchemaNestedBlockFromProto(nb *planproto.SchemaNestedBlock) *configschema.NestedBlock {
	block := configSchemaBlockFromProto(nb.Block)
	return &configschema.NestedBlock{
		Block:    *block,
		Nesting:  nestingModeFromProto(nb.Nesting),
		MinItems: int(nb.MinItems),
		MaxItems: int(nb.MaxItems),
	}
}

func nestingModeToProto(m configschema.NestingMode) planproto.SchemaNestingMode {
	switch m {
	case configschema.NestingSingle:
		return planproto.SchemaNestingMode_SCHEMA_NESTING_SINGLE
	case configschema.NestingGroup:
		return planproto.SchemaNestingMode_SCHEMA_NESTING_GROUP
	case configschema.NestingList:
		return planproto.SchemaNestingMode_SCHEMA_NESTING_LIST
	case configschema.NestingSet:
		return planproto.SchemaNestingMode_SCHEMA_NESTING_SET
	case configschema.NestingMap:
		return planproto.SchemaNestingMode_SCHEMA_NESTING_MAP
	default:
		return planproto.SchemaNestingMode_SCHEMA_NESTING_INVALID
	}
}

func nestingModeFromProto(m planproto.SchemaNestingMode) configschema.NestingMode {
	switch m {
	case planproto.SchemaNestingMode_SCHEMA_NESTING_SINGLE:
		return configschema.NestingSingle
	case planproto.SchemaNestingMode_SCHEMA_NESTING_GROUP:
		return configschema.NestingGroup
	case planproto.SchemaNestingMode_SCHEMA_NESTING_LIST:
		return configschema.NestingList
	case planproto.SchemaNestingMode_SCHEMA_NESTING_SET:
		return configschema.NestingSet
	case planproto.SchemaNestingMode_SCHEMA_NESTING_MAP:
		return configschema.NestingMap
	default:
		// Leave as the zero value (invalid) and let the caller validate
		// and deal with this, matching the behavior of the equivalent
		// plugin protocol conversions.
		return 0
	}
}

// providerSchemaToProto converts a single (already-trimmed) provider schema
// into its protobuf representation.
func providerSchemaToProto(s providers.ProviderSchema) *planproto.ProviderSchema {
	ps := &planproto.ProviderSchema{
		ProviderConfig:       schemaToProto(s.Provider),
		ManagedResourceTypes: make(map[string]*planproto.ResourceSchema, len(s.ResourceTypes)),
		DataSources:          make(map[string]*planproto.ResourceSchema, len(s.DataSources)),
	}
	for name, schema := range s.ResourceTypes {
		ps.ManagedResourceTypes[name] = resourceSchemaToProto(schema)
	}
	for name, schema := range s.DataSources {
		ps.DataSources[name] = resourceSchemaToProto(schema)
	}
	return ps
}

func providerSchemaFromProto(ps *planproto.ProviderSchema) providers.ProviderSchema {
	s := providers.ProviderSchema{
		Provider:      schemaFromProto(ps.ProviderConfig),
		ResourceTypes: make(map[string]providers.Schema, len(ps.ManagedResourceTypes)),
		DataSources:   make(map[string]providers.Schema, len(ps.DataSources)),
	}
	for name, rs := range ps.ManagedResourceTypes {
		s.ResourceTypes[name] = resourceSchemaFromProto(rs)
	}
	for name, rs := range ps.DataSources {
		s.DataSources[name] = resourceSchemaFromProto(rs)
	}
	return s
}
