// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/states"
)

// GetResourceSchema extracts a resource schema from the schemas parameter
// using reflection since we can't import tofu.Schemas directly
func GetResourceSchema(schemas interface{}, provider addrs.Provider, mode addrs.ResourceMode, typeName string) (*configschema.Block, uint64) {
	if schemas == nil {
		return nil, 0
	}

	// Use reflection to call ResourceTypeConfig method
	v := reflect.ValueOf(schemas)
	method := v.MethodByName("ResourceTypeConfig")
	if !method.IsValid() {
		return nil, 0
	}

	// Call the method
	args := []reflect.Value{
		reflect.ValueOf(provider),
		reflect.ValueOf(mode),
		reflect.ValueOf(typeName),
	}
	results := method.Call(args)
	
	if len(results) != 2 {
		return nil, 0
	}

	// Extract the schema
	if results[0].IsNil() {
		return nil, 0
	}
	
	schema, _ := results[0].Interface().(*configschema.Block)
	version := results[1].Interface().(uint64)
	
	return schema, version
}

// ContainsSensitive checks if a schema contains sensitive attributes
func ContainsSensitive(schema *configschema.Block) bool {
	if schema == nil {
		return false
	}
	return schema.ContainsSensitive()
}

// GetValueMarks retrieves marks from a schema
func GetValueMarks(schema *configschema.Block, val cty.Value, path cty.Path) []cty.PathValueMarks {
	if schema == nil {
		return nil
	}
	return schema.ValueMarks(val, path)
}

// SensitiveAsBoolWithPathValueMarks returns a value with the same structure as the given value,
// but with all sensitive marks replaced with true and all non-sensitive leaf values omitted.
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

// MarshalCheckStates handles check state marshaling
// This is a placeholder - we'll need to implement this based on jsonchecks
func MarshalCheckStates(checks interface{}) json.RawMessage {
	// Placeholder implementation
	return json.RawMessage("{}")
}

// MarshalState handles state marshaling
func MarshalState(sf interface{}, schemas interface{}) (json.RawMessage, error) {
	if sf == nil {
		return json.RawMessage("{}"), nil
	}

	// Use reflection to access the State field from the statefile
	v := reflect.ValueOf(sf).Elem()
	stateField := v.FieldByName("State")
	if !stateField.IsValid() || stateField.IsNil() {
		return json.RawMessage("{}"), nil
	}

	state := stateField.Interface().(*states.State)

	// Create a JSONStateValues structure
	sv := &JSONStateValues{}

	// Marshal outputs
	var err error
	sv.Outputs, err = marshalStateOutputs(state.RootModule().OutputValues)
	if err != nil {
		return nil, err
	}

	// Marshal root module and child modules
	sv.RootModule, err = marshalStateRootModule(state, schemas)
	if err != nil {
		return nil, err
	}

	// Convert to JSON
	result, err := json.Marshal(sv)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(result), nil
}

// marshalStateOutputs converts state output values to JSON format
func marshalStateOutputs(outputs map[string]*states.OutputValue) (map[string]JSONOutput, error) {
	if outputs == nil {
		return nil, nil
	}

	ret := make(map[string]JSONOutput)
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
		ret[k] = JSONOutput{
			Value:      ov,
			Type:       ot,
			Sensitive:  v.Sensitive,
			Deprecated: v.Deprecated,
		}
	}

	return ret, nil
}

func marshalStateRootModule(s *states.State, schemas interface{}) (*JSONModule, error) {
	var ret JSONModule
	var err error

	ret.Address = ""
	rs, err := marshalStateResources(s.RootModule().Resources, addrs.RootModuleInstance, schemas)
	if err != nil {
		return &ret, err
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
				return &ret, diags.Err()
			}
			moduleMap[parent] = append(moduleMap[parent], childModuleInstance)
		}
	}

	// use the state and module map to build up the module structure
	ret.ChildModules, err = marshalStateModules(s, schemas, moduleMap[""], moduleMap)
	return &ret, err
}

// marshalStateModules is a recursive function to build a module structure
// out of tofu state.
func marshalStateModules(
	s *states.State,
	schemas interface{},
	modules []addrs.ModuleInstance,
	moduleMap map[string][]addrs.ModuleInstance,
) ([]JSONModule, error) {
	var ret []JSONModule
	for _, child := range modules {
		// cm for child module, naming things is hard.
		cm := JSONModule{Address: child.String()}

		// the module may be resourceless and contain only submodules, it will then be nil here
		stateMod := s.Module(child)
		if stateMod != nil {
			rs, err := marshalStateResources(stateMod.Resources, stateMod.Addr, schemas)
			if err != nil {
				return nil, err
			}
			cm.Resources = rs
		}

		if moduleMap[child.String()] != nil {
			moreChildModules, err := marshalStateModules(s, schemas, moduleMap[child.String()], moduleMap)
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

func marshalStateResources(resources map[string]*states.Resource, module addrs.ModuleInstance, schemas interface{}) ([]JSONResource, error) {
	var ret []JSONResource

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

			current := JSONResource{
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
				current.Mode = "managed"
			case addrs.DataResourceMode:
				current.Mode = "data"
			default:
				return ret, fmt.Errorf("resource %s has an unsupported mode %s",
					resAddr.String(),
					resAddr.Mode.String(),
				)
			}

			schema, version := GetResourceSchema(schemas, r.ProviderConfig.Provider, resAddr.Mode, resAddr.Type)

			// It is possible that the only instance is deposed
			if ri.Current != nil {
				if schema != nil && version != ri.Current.SchemaVersion {
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
				if ContainsSensitive(schema) {
					marks = append(marks, GetValueMarks(schema, value, nil)...)
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
				deposed := JSONResource{
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
				if ContainsSensitive(schema) {
					marks = append(marks, GetValueMarks(schema, value, nil)...)
				}
				s := SensitiveAsBoolWithPathValueMarks(value, marks)
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

// marshalAttributeValues creates a JSON representation of attribute values
func marshalAttributeValues(value cty.Value) json.RawMessage {
	// unmark our value to show all values
	value, _ = value.UnmarkDeep()

	if value == cty.NilVal || value.IsNull() {
		return nil
	}

	ret := make(map[string]json.RawMessage)

	it := value.ElementIterator()
	for it.Next() {
		k, v := it.Element()
		vJSON, _ := ctyjson.Marshal(v, v.Type())
		ret[k.AsString()] = json.RawMessage(vJSON)
	}
	
	// Convert to JSON
	result, _ := json.Marshal(ret)
	return json.RawMessage(result)
}

// MarshalConfig handles config marshaling
// This is a placeholder - we'll need to implement this based on jsonconfig
func MarshalConfig(config *configs.Config, schemas interface{}) (json.RawMessage, error) {
	// Placeholder implementation
	return json.RawMessage("{}"), nil
}