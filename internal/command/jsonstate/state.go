// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonstate

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/jsonchecks"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/tofu"
)

const (
	// FormatVersion represents the version of the json format and will be
	// incremented for any change to this format that requires changes to a
	// consuming parser.
	FormatVersion = "1.0"

	ManagedResourceMode = "managed"
	DataResourceMode    = "data"
)

// State is the top-level representation of the json format of a tofu
// state.
type State struct {
	FormatVersion    string          `json:"format_version,omitempty"`
	TerraformVersion string          `json:"terraform_version,omitempty"`
	Values           *StateValues    `json:"values,omitempty"`
	Checks           json.RawMessage `json:"checks,omitempty"`
}

// StateValues is the common representation of resolved values for both the prior
// state (which is always complete) and the planned new state.
type StateValues struct {
	Outputs    map[string]Output `json:"outputs,omitempty"`
	RootModule Module            `json:"root_module,omitempty"`
}

type Output struct {
	Sensitive  bool            `json:"sensitive"`
	Deprecated string          `json:"deprecated,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
	Type       json.RawMessage `json:"type,omitempty"`
}

// Module is the representation of a module in state. This can be the root module
// or a child module
type Module struct {
	// Resources are sorted in a user-friendly order that is undefined at this
	// time, but consistent.
	Resources []Resource `json:"resources,omitempty"`

	// Address is the absolute module address, omitted for the root module
	Address string `json:"address,omitempty"`

	// Each module object can optionally have its own nested "child_modules",
	// recursively describing the full module tree.
	ChildModules []Module `json:"child_modules,omitempty"`
}

// Resource is the representation of a resource in the state.
type Resource struct {
	// Address is the absolute resource address
	Address string `json:"address,omitempty"`

	// Mode can be "managed" or "data"
	Mode string `json:"mode,omitempty"`

	Type string `json:"type,omitempty"`
	Name string `json:"name,omitempty"`

	// Index is omitted for a resource not using `count` or `for_each`.
	Index json.RawMessage `json:"index,omitempty"`

	// ProviderName allows the property "type" to be interpreted unambiguously
	// in the unusual situation where a provider offers a resource type whose
	// name does not start with its own name, such as the "googlebeta" provider
	// offering "google_compute_instance".
	ProviderName string `json:"provider_name"`

	// SchemaVersion indicates which version of the resource type schema the
	// "values" property conforms to.
	SchemaVersion uint64 `json:"schema_version"`

	// AttributeValues is the JSON representation of the attribute values of the
	// resource, whose structure depends on the resource type schema. Any
	// unknown values are omitted or set to null, making them indistinguishable
	// from absent values.
	AttributeValues AttributeValues `json:"values,omitempty"`

	// SensitiveValues is similar to AttributeValues, but with all sensitive
	// values replaced with true, and all non-sensitive leaf values omitted.
	SensitiveValues json.RawMessage `json:"sensitive_values,omitempty"`

	// DependsOn contains a list of the resource's dependencies. The entries are
	// addresses relative to the containing module.
	DependsOn []string `json:"depends_on,omitempty"`

	// Tainted is true if the resource is tainted in tofu state.
	Tainted bool `json:"tainted,omitempty"`

	// Deposed is set if the resource is deposed in tofu state.
	DeposedKey string `json:"deposed_key,omitempty"`
}

// AttributeValues is the JSON representation of the attribute values of the
// resource, whose structure depends on the resource type schema.
type AttributeValues map[string]json.RawMessage

func marshalAttributeValues(value cty.Value) AttributeValues {
	// unmark our value to show all values
	value, _ = value.UnmarkDeep()

	if value == cty.NilVal || value.IsNull() {
		return nil
	}

	ret := make(AttributeValues)

	it := value.ElementIterator()
	for it.Next() {
		k, v := it.Element()
		vJSON, _ := ctyjson.Marshal(v, v.Type())
		ret[k.AsString()] = json.RawMessage(vJSON)
	}
	return ret
}

// newState() returns a minimally-initialized state
func newState() *State {
	return &State{
		FormatVersion: FormatVersion,
	}
}

// MarshalForRenderer returns the pre-json encoding changes of the state, in a
// format available to the structured renderer.
func MarshalForRenderer(sf *statefile.File, schemas *tofu.Schemas) (Module, map[string]Output, error) {
	if sf.State.Modules == nil {
		// Empty state case.
		return Module{}, nil, nil
	}

	outputs, err := MarshalOutputs(sf.State.RootModule().OutputValues)
	if err != nil {
		return Module{}, nil, err
	}

	root, err := marshalRootModule(sf.State, schemas)
	if err != nil {
		return Module{}, nil, err
	}

	return root, outputs, err
}

// MarshalForLog returns the origin JSON compatible state, read for a logging
// package to marshal further.
func MarshalForLog(sf *statefile.File, schemas *tofu.Schemas) (*State, error) {
	output := newState()

	if sf == nil || sf.State.Empty() {
		return output, nil
	}

	if sf.TerraformVersion != nil {
		output.TerraformVersion = sf.TerraformVersion.String()
	}

	// output.StateValues
	err := output.marshalStateValues(sf.State, schemas)
	if err != nil {
		return nil, err
	}

	// output.Checks
	if sf.State.CheckResults != nil && sf.State.CheckResults.ConfigResults.Len() > 0 {
		output.Checks = jsonchecks.MarshalCheckStates(sf.State.CheckResults)
	}

	return output, nil
}

// Marshal returns the json encoding of a tofu state.
func Marshal(sf *statefile.File, schemas *tofu.Schemas) ([]byte, error) {
	output, err := MarshalForLog(sf, schemas)
	if err != nil {
		return nil, err
	}

	ret, err := json.Marshal(output)
	return ret, err
}

func (jsonstate *State) marshalStateValues(s *states.State, schemas *tofu.Schemas) error {
	var sv StateValues
	var err error

	// only marshal the root module outputs
	sv.Outputs, err = MarshalOutputs(s.RootModule().OutputValues)
	if err != nil {
		return err
	}

	// use the state and module map to build up the module structure
	sv.RootModule, err = marshalRootModule(s, schemas)
	if err != nil {
		return err
	}

	jsonstate.Values = &sv
	return nil
}

// MarshalOutputs translates a map of states.OutputValue to a map of jsonstate.Output,
// which are defined for json encoding.
func MarshalOutputs(outputs map[string]*states.OutputValue) (map[string]Output, error) {
	if outputs == nil {
		return nil, nil
	}

	ret := make(map[string]Output)
	for k, v := range outputs {
		ty := v.Value.Type()
		ov, err := ctyjson.Marshal(v.Value, ty)
		if err != nil {
			return ret, err
		}
		ot, err := ctyjson.MarshalType(ty)
		if err != nil {
			return ret, err
		}
		ret[k] = Output{
			Value:      ov,
			Type:       ot,
			Sensitive:  v.Sensitive,
			Deprecated: v.Deprecated,
		}
	}

	return ret, nil
}

func marshalRootModule(s *states.State, schemas *tofu.Schemas) (Module, error) {
	var ret Module
	var err error

	ret.Address = ""
	rs, err := marshalResources(s.RootModule().Resources, addrs.RootModuleInstance, schemas)
	if err != nil {
		return ret, err
	}
	ret.Resources = rs

	// build a map of module -> set[child module addresses]
	moduleChildSet := make(map[string]map[string]struct{})
	for _, mod := range s.Modules {
		if mod.Addr.IsRoot() {
			continue
		} else {
			for childAddr := mod.Addr; !childAddr.IsRoot(); childAddr = childAddr.Parent() {
				if _, ok := moduleChildSet[childAddr.Parent().String()]; !ok {
					moduleChildSet[childAddr.Parent().String()] = map[string]struct{}{}
				}
				moduleChildSet[childAddr.Parent().String()][childAddr.String()] = struct{}{}
			}
		}
	}

	// transform the previous map into map of module -> [child module addresses]
	moduleMap := make(map[string][]addrs.ModuleInstance)
	for parent, children := range moduleChildSet {
		for child := range children {
			childModuleInstance, diags := addrs.ParseModuleInstanceStr(child)
			if diags.HasErrors() {
				return ret, diags.Err()
			}
			moduleMap[parent] = append(moduleMap[parent], childModuleInstance)
		}
	}

	// use the state and module map to build up the module structure
	ret.ChildModules, err = marshalModules(s, schemas, moduleMap[""], moduleMap)
	return ret, err
}

// marshalModules is an ungainly recursive function to build a module structure
// out of tofu state.
func marshalModules(
	s *states.State,
	schemas *tofu.Schemas,
	modules []addrs.ModuleInstance,
	moduleMap map[string][]addrs.ModuleInstance,
) ([]Module, error) {
	var ret []Module
	for _, child := range modules {
		// cm for child module, naming things is hard.
		cm := Module{Address: child.String()}

		// the module may be resourceless and contain only submodules, it will then be nil here
		stateMod := s.Module(child)
		if stateMod != nil {
			rs, err := marshalResources(stateMod.Resources, stateMod.Addr, schemas)
			if err != nil {
				return nil, err
			}
			cm.Resources = rs
		}

		if moduleMap[child.String()] != nil {
			moreChildModules, err := marshalModules(s, schemas, moduleMap[child.String()], moduleMap)
			if err != nil {
				return nil, err
			}
			cm.ChildModules = moreChildModules
		}

		ret = append(ret, cm)
	}

	// sort the child modules by address for consistency.
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Address < ret[j].Address
	})

	return ret, nil
}

func marshalResources(resources map[string]*states.Resource, module addrs.ModuleInstance, schemas *tofu.Schemas) ([]Resource, error) {
	var ret []Resource

	var sortedResources []*states.Resource
	for _, r := range resources {
		sortedResources = append(sortedResources, r)
	}
	sort.Slice(sortedResources, func(i, j int) bool {
		return sortedResources[i].Addr.Less(sortedResources[j].Addr)
	})

	for _, r := range sortedResources {

		var sortedKeys []addrs.InstanceKey
		for k := range r.Instances {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Slice(sortedKeys, func(i, j int) bool {
			return addrs.InstanceKeyLess(sortedKeys[i], sortedKeys[j])
		})

		for _, k := range sortedKeys {
			ri := r.Instances[k]

			var err error

			resAddr := r.Addr.Resource

			current := Resource{
				Address:      r.Addr.Instance(k).String(),
				Type:         resAddr.Type,
				Name:         resAddr.Name,
				ProviderName: r.ProviderConfig.Provider.String(),
			}

			if k != nil {
				index := k.Value()
				if current.Index, err = ctyjson.Marshal(index, index.Type()); err != nil {
					return nil, err
				}
			}

			switch resAddr.Mode {
			case addrs.ManagedResourceMode:
				current.Mode = ManagedResourceMode
			case addrs.DataResourceMode:
				current.Mode = DataResourceMode
			default:
				return ret, fmt.Errorf("resource %s has an unsupported mode %s",
					resAddr.String(),
					resAddr.Mode.String(),
				)
			}

			schema, version := schemas.ResourceTypeConfig(
				r.ProviderConfig.Provider,
				resAddr.Mode,
				resAddr.Type,
			)

			// It is possible that the only instance is deposed
			if ri.Current != nil {
				if version != ri.Current.SchemaVersion {
					return nil, fmt.Errorf("schema version %d for %s in state does not match version %d from the provider", ri.Current.SchemaVersion, resAddr, version)
				}

				current.SchemaVersion = ri.Current.SchemaVersion

				if schema == nil {
					return nil, fmt.Errorf("no schema found for %s (in provider %s)", resAddr.String(), r.ProviderConfig.Provider)
				}
				riObj, err := ri.Current.Decode(schema.ImpliedType())
				if err != nil {
					return nil, err
				}

				current.AttributeValues = marshalAttributeValues(riObj.Value)

				value, marks := riObj.Value.UnmarkDeepWithPaths()
				if schema.ContainsSensitive() {
					marks = append(marks, schema.ValueMarks(value, nil)...)
				}
				s := SensitiveAsBoolWithPathValueMarks(value, marks)
				v, err := ctyjson.Marshal(s, s.Type())
				if err != nil {
					return nil, err
				}
				current.SensitiveValues = v

				if len(riObj.Dependencies) > 0 {
					dependencies := make([]string, len(riObj.Dependencies))
					for i, v := range riObj.Dependencies {
						dependencies[i] = v.String()
					}
					current.DependsOn = dependencies
				}

				if riObj.Status == states.ObjectTainted {
					current.Tainted = true
				}
				ret = append(ret, current)
			}

			var sortedDeposedKeys []string
			for k := range ri.Deposed {
				sortedDeposedKeys = append(sortedDeposedKeys, string(k))
			}
			sort.Strings(sortedDeposedKeys)

			for _, deposedKey := range sortedDeposedKeys {
				rios := ri.Deposed[states.DeposedKey(deposedKey)]

				// copy the base fields from the current instance
				deposed := Resource{
					Address:      current.Address,
					Type:         current.Type,
					Name:         current.Name,
					ProviderName: current.ProviderName,
					Mode:         current.Mode,
					Index:        current.Index,
				}

				riObj, err := rios.Decode(schema.ImpliedType())
				if err != nil {
					return nil, err
				}

				deposed.AttributeValues = marshalAttributeValues(riObj.Value)

				value, marks := riObj.Value.UnmarkDeepWithPaths()
				if schema.ContainsSensitive() {
					marks = append(marks, schema.ValueMarks(value, nil)...)
				}
				s := SensitiveAsBool(value.MarkWithPaths(marks))
				v, err := ctyjson.Marshal(s, s.Type())
				if err != nil {
					return nil, err
				}
				deposed.SensitiveValues = v

				if len(riObj.Dependencies) > 0 {
					dependencies := make([]string, len(riObj.Dependencies))
					for i, v := range riObj.Dependencies {
						dependencies[i] = v.String()
					}
					deposed.DependsOn = dependencies
				}

				if riObj.Status == states.ObjectTainted {
					deposed.Tainted = true
				}
				deposed.DeposedKey = deposedKey
				ret = append(ret, deposed)
			}
		}
	}

	return ret, nil
}

func SensitiveAsBool(val cty.Value) cty.Value {
	return SensitiveAsBoolWithPathValueMarks(val, nil)
}

func SensitiveAsBoolWithPathValueMarks(val cty.Value, pvms []cty.PathValueMarks) cty.Value {
	var sensitiveMarks []cty.PathValueMarks
	for _, pvm := range pvms {
		if _, ok := pvm.Marks[marks.Sensitive]; ok {
			sensitiveMarks = append(sensitiveMarks, pvm)
		}
	}
	return sensitiveAsBoolWithPathValueMarks(val, cty.Path{}, sensitiveMarks)
}

func sensitiveAsBoolWithPathValueMarks(val cty.Value, path cty.Path, pvms []cty.PathValueMarks) cty.Value {
	if val.HasMark(marks.Sensitive) {
		return cty.True
	}
	for _, pvm := range pvms {
		if path.Equals(pvm.Path) {
			return cty.True
		}
	}
	ty := val.Type()
	switch {
	case val.IsNull(), ty.IsPrimitiveType(), ty.Equals(cty.DynamicPseudoType):
		return cty.False
	case ty.IsListType() || ty.IsTupleType() || ty.IsSetType():
		return sensitiveCollectionAsBool(val, path, pvms)
	case ty.IsMapType():
		return sensitiveMapAsBool(val, path, pvms)
	case ty.IsObjectType():
		return sensitiveObjectAsBool(val, path, pvms)
	default:
		// Should never happen, since the above should cover all types
		panic(fmt.Sprintf("sensitiveAsBoolWithPathValueMarks cannot handle %#v", val))
	}
}

func sensitiveCollectionAsBool(val cty.Value, path []cty.PathStep, pvms []cty.PathValueMarks) cty.Value {
	if !val.IsKnown() {
		// If the collection is unknown we can't say anything about the
		// sensitivity of its contents
		return cty.EmptyTupleVal
	}
	length := val.LengthInt()
	if length == 0 {
		// If there are no elements then we can't have sensitive values
		return cty.EmptyTupleVal
	}
	vals := make([]cty.Value, 0, length)
	it := val.ElementIterator()
	for it.Next() {
		kv, ev := it.Element()
		path = append(path, cty.IndexStep{
			Key: kv,
		})
		vals = append(vals, sensitiveAsBoolWithPathValueMarks(ev, path, pvms))
		path = path[0 : len(path)-1]
	}
	// The above transform may have changed the types of some of the
	// elements, so we'll always use a tuple here in case we've now made
	// different elements have different types. Our ultimate goal is to
	// marshal to JSON anyway, and all of these sequence types are
	// indistinguishable in JSON.
	return cty.TupleVal(vals)
}

func sensitiveMapAsBool(val cty.Value, path []cty.PathStep, pvms []cty.PathValueMarks) cty.Value {
	if !val.IsKnown() {
		// If the map/object is unknown we can't say anything about the
		// sensitivity of its attributes
		return cty.EmptyObjectVal
	}
	length := val.LengthInt()
	if length == 0 {
		// If there are no elements then we can't have sensitive values
		return cty.EmptyObjectVal
	}

	vals := make(map[string]cty.Value)
	it := val.ElementIterator()
	for it.Next() {
		kv, ev := it.Element()
		path = append(path, cty.IndexStep{
			Key: kv,
		})
		s := sensitiveAsBoolWithPathValueMarks(ev, path, pvms)
		path = path[0 : len(path)-1]
		// Omit all of the "false"s for non-sensitive values for more
		// compact serialization
		if !s.RawEquals(cty.False) {
			vals[kv.AsString()] = s
		}
	}
	// The above transform may have changed the types of some of the
	// elements, so we'll always use an object here in case we've now made
	// different elements have different types. Our ultimate goal is to
	// marshal to JSON anyway, and all of these mapping types are
	// indistinguishable in JSON.
	return cty.ObjectVal(vals)
}

func sensitiveObjectAsBool(val cty.Value, path []cty.PathStep, pvms []cty.PathValueMarks) cty.Value {
	if !val.IsKnown() {
		// If the map/object is unknown we can't say anything about the
		// sensitivity of its attributes
		return cty.EmptyObjectVal
	}
	ty := val.Type()
	if len(ty.AttributeTypes()) == 0 {
		// If there are no elements then we can't have sensitive values
		return cty.EmptyObjectVal
	}
	vals := make(map[string]cty.Value)
	for name := range ty.AttributeTypes() {
		av := val.GetAttr(name)
		path = append(path, cty.GetAttrStep{
			Name: name,
		})
		s := sensitiveAsBoolWithPathValueMarks(av, path, pvms)
		path = path[0 : len(path)-1]
		if !s.RawEquals(cty.False) {
			vals[name] = s
		}
	}
	return cty.ObjectVal(vals)
}
