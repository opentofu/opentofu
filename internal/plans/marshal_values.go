// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

func (p *JSONPlan) marshalPlannedValues(changes *Changes, schemas interface{}) error {
	// marshal the planned changes into a module
	plan, err := marshalPlannedValues(changes, schemas)
	if err != nil {
		return err
	}
	p.PlannedValues.RootModule = plan

	// marshalPlannedOutputs
	outputs, err := marshalPlannedOutputs(changes)
	if err != nil {
		return err
	}
	p.PlannedValues.Outputs = outputs

	return nil
}

// marshalPlannedValues extracts the planned values from a set of changes
func marshalPlannedValues(changes *Changes, schemas interface{}) (*JSONModule, error) {
	if changes == nil {
		return nil, nil
	}

	// Group changes by module
	moduleChanges := make(map[string][]*ResourceInstanceChangeSrc)
	for _, rc := range changes.Resources {
		moduleKey := rc.Addr.Module.String()
		moduleChanges[moduleKey] = append(moduleChanges[moduleKey], rc)
	}

	// Start with root module
	rootModule := &JSONModule{
		Address: "",
	}

	// Process root module resources
	if resources, ok := moduleChanges[""]; ok {
		marshaledResources := make([]JSONResource, 0, len(resources))
		for _, rc := range resources {
			if rc.Action == Delete {
				continue
			}

			addr := rc.Addr
			schema, _ := GetResourceSchema(schemas, rc.ProviderAddr.Provider, addr.Resource.Resource.Mode, addr.Resource.Resource.Type)
			if schema == nil {
				return nil, fmt.Errorf("no schema found for %s", addr)
			}

			changeV, err := rc.Decode(schema.ImpliedType())
			if err != nil {
				return nil, err
			}

			// We drop the marks from the change, as decoding is only an
			// intermediate step to re-encode the values as json
			changeV.After, _ = changeV.After.UnmarkDeep()

			if changeV.After == cty.NilVal {
				continue
			}

			var after []byte
			if changeV.After.IsWhollyKnown() {
				after, err = ctyjson.Marshal(changeV.After, changeV.After.Type())
				if err != nil {
					return nil, err
				}
			} else {
				filteredAfter := OmitUnknowns(changeV.After)
				if filteredAfter.IsNull() {
					after = nil
				} else {
					after, err = ctyjson.Marshal(filteredAfter, filteredAfter.Type())
					if err != nil {
						return nil, err
					}
				}
			}

			marks := rc.AfterValMarks
			if ContainsSensitive(schema) {
				marks = append(marks, GetValueMarks(schema, changeV.After, nil)...)
			}
			as := SensitiveAsBoolWithPathValueMarks(changeV.After, marks)
			afterSensitive, err := ctyjson.Marshal(as, as.Type())
			if err != nil {
				return nil, err
			}

			r := JSONResource{
				Address:          addr.String(),
				Mode:             addr.Resource.Resource.Mode.String(),
				Type:             addr.Resource.Resource.Type,
				Name:             addr.Resource.Resource.Name,
				ProviderName:     rc.ProviderAddr.Provider.String(),
				AttributeValues:  json.RawMessage(after),
				SensitiveValues:  json.RawMessage(afterSensitive),
			}

			key := addr.Resource.Key
			if key != nil {
				value := key.Value()
				r.Index, err = ctyjson.Marshal(value, value.Type())
				if err != nil {
					return nil, err
				}
			}

			marshaledResources = append(marshaledResources, r)
		}

		// Sort resources by address for consistency
		sort.Slice(marshaledResources, func(i, j int) bool {
			return marshaledResources[i].Address < marshaledResources[j].Address
		})

		rootModule.Resources = marshaledResources
	}

	// Process child modules
	var childModules []JSONModule
	for moduleAddr, resources := range moduleChanges {
		if moduleAddr == "" {
			continue // Skip root module, already processed
		}

		childModule := JSONModule{
			Address: moduleAddr,
		}

		marshaledResources := make([]JSONResource, 0, len(resources))
		for _, rc := range resources {
			if rc.Action == Delete {
				continue
			}

			addr := rc.Addr
			schema, _ := GetResourceSchema(schemas, rc.ProviderAddr.Provider, addr.Resource.Resource.Mode, addr.Resource.Resource.Type)
			if schema == nil {
				continue
			}

			changeV, err := rc.Decode(schema.ImpliedType())
			if err != nil {
				continue
			}

			changeV.After, _ = changeV.After.UnmarkDeep()
			if changeV.After == cty.NilVal {
				continue
			}

			var after []byte
			if changeV.After.IsWhollyKnown() {
				after, err = ctyjson.Marshal(changeV.After, changeV.After.Type())
				if err != nil {
					continue
				}
			} else {
				filteredAfter := OmitUnknowns(changeV.After)
				if !filteredAfter.IsNull() {
					after, err = ctyjson.Marshal(filteredAfter, filteredAfter.Type())
					if err != nil {
						continue
					}
				}
			}

			r := JSONResource{
				Address:         addr.String(),
				Mode:            addr.Resource.Resource.Mode.String(),
				Type:            addr.Resource.Resource.Type,
				Name:            addr.Resource.Resource.Name,
				ProviderName:    rc.ProviderAddr.Provider.String(),
				AttributeValues: json.RawMessage(after),
			}

			marshaledResources = append(marshaledResources, r)
		}

		if len(marshaledResources) > 0 {
			childModule.Resources = marshaledResources
			childModules = append(childModules, childModule)
		}
	}

	// Sort child modules for consistency
	sort.Slice(childModules, func(i, j int) bool {
		return childModules[i].Address < childModules[j].Address
	})

	if len(childModules) > 0 {
		rootModule.ChildModules = childModules
	}

	return rootModule, nil
}

// marshalPlannedOutputs marshals the planned outputs from changes
func marshalPlannedOutputs(changes *Changes) (map[string]JSONOutput, error) {
	if changes == nil || len(changes.Outputs) == 0 {
		return nil, nil
	}

	outputs := make(map[string]JSONOutput)

	for _, oc := range changes.Outputs {
		if !oc.Addr.Module.IsRoot() {
			continue
		}

		if oc.Action == Delete {
			continue
		}

		changeV, err := oc.Decode()
		if err != nil {
			return nil, err
		}

		// We drop the marks from the change, as decoding is only an
		// intermediate step to re-encode the values as json
		changeV.After, _ = changeV.After.UnmarkDeep()

		if changeV.After == cty.NilVal {
			continue
		}

		var after []byte
		if changeV.After.IsWhollyKnown() {
			after, err = ctyjson.Marshal(changeV.After, changeV.After.Type())
			if err != nil {
				return nil, err
			}
		} else {
			filteredAfter := OmitUnknowns(changeV.After)
			if !filteredAfter.IsNull() {
				after, err = ctyjson.Marshal(filteredAfter, filteredAfter.Type())
				if err != nil {
					return nil, err
				}
			}
		}

		// Encode the type
		ty := changeV.After.Type()
		tyJSON, err := ctyjson.MarshalType(ty)
		if err != nil {
			return nil, err
		}

		output := JSONOutput{
			Value:     json.RawMessage(after),
			Type:      json.RawMessage(tyJSON),
			Sensitive: oc.Sensitive,
		}

		outputs[oc.Addr.OutputValue.Name] = output
	}

	return outputs, nil
}