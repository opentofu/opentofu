// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planfile

import (
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plans/internal/planproto"
	"github.com/opentofu/opentofu/internal/providers"
)

const (
	tfschemasFilename = "tfschemas"
)

// ErrNoSchemas is returned by Reader.ReadSchemas when the plan file does
// not have a schemas entry at all
var ErrNoSchemas = errors.New("plan file does not have a stored schemas entry")

// prepareSchemasForWrite computes the trimmed-down set of provider schemas
// that are needed to render the given plan, from the full set of schemas
// that were used to create it.
//
// trimSchemas already guarantees an all-or-nothing result (nil if it can't
// fully cover the plan), so there's nothing left for this function to
// validate; it just handles the nil-plan/nil-schemas cases that don't make
// sense to pass through at all.
func prepareSchemasForWrite(plan *plans.Plan, config *configs.Config, schemas map[addrs.Provider]providers.ProviderSchema) map[addrs.Provider]providers.ProviderSchema {
	if plan == nil || schemas == nil {
		return nil
	}

	return trimSchemas(plan, config, schemas)
}

// writeSchemas writes the given (already-trimmed) provider schemas to w
// in the protobuf encoding used for the "tfschemas" entry of a plan file.
func writeSchemas(schemas map[addrs.Provider]providers.ProviderSchema, w io.Writer) error {
	raw := &planproto.Schemas{
		Providers: make(map[string]*planproto.ProviderSchema, len(schemas)),
	}
	for provider, schema := range schemas {
		raw.Providers[provider.String()] = providerSchemaToProto(schema)
	}

	src, err := proto.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to marshal schemas: %w", err)
	}

	_, err = w.Write(src)
	return err
}

// readSchemas reads and decodes the "tfschemas" entry of a plan file.
func readSchemas(r io.Reader) (map[addrs.Provider]providers.ProviderSchema, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var raw planproto.Schemas
	if err := proto.Unmarshal(src, &raw); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	ret := make(map[addrs.Provider]providers.ProviderSchema, len(raw.Providers))
	for providerStr, ps := range raw.Providers {
		provider, diags := addrs.ParseProviderSourceString(providerStr)
		if diags.HasErrors() {
			return nil, fmt.Errorf("invalid provider address %q in stored schemas: %w", providerStr, diags.Err())
		}
		ret[provider] = providerSchemaFromProto(ps)
	}

	return ret, nil
}
