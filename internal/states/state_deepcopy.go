// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package states

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty"
)

// Taking deep copies of states is an important operation because state is
// otherwise a mutable data structure that is challenging to share across
// many separate callers. It is important that the DeepCopy implementations
// in this file comprehensively copy all parts of the state data structure
// that could be mutated via pointers.

// DeepCopy returns a new state that contains equivalent data to the receiver
// but shares no backing memory in common.
//
// As with all methods on State, this method is not safe to use concurrently
// with writing to any portion of the receiving data structure. It is the
// caller's responsibility to ensure mutual exclusion for the duration of the
// operation, but may then freely modify the receiver and the returned copy
// independently once this method returns.
func (s *State) DeepCopy() *State {
	if s == nil {
		return nil
	}

	modules := make(map[string]*Module, len(s.Modules))
	for k, m := range s.Modules {
		modules[k] = m.DeepCopy()
	}
	return &State{
		Modules:      modules,
		CheckResults: s.CheckResults.DeepCopy(),
	}
}

// DeepCopy returns a new module state that contains equivalent data to the
// receiver but shares no backing memory in common.
//
// As with all methods on Module, this method is not safe to use concurrently
// with writing to any portion of the receiving data structure. It is the
// caller's responsibility to ensure mutual exclusion for the duration of the
// operation, but may then freely modify the receiver and the returned copy
// independently once this method returns.
func (ms *Module) DeepCopy() *Module {
	if ms == nil {
		return nil
	}

	resources := make(map[string]*Resource, len(ms.Resources))
	for k, r := range ms.Resources {
		resources[k] = r.DeepCopy()
	}
	outputValues := make(map[string]*OutputValue, len(ms.OutputValues))
	for k, v := range ms.OutputValues {
		outputValues[k] = v.DeepCopy()
	}
	localValues := make(map[string]cty.Value, len(ms.LocalValues))
	for k, v := range ms.LocalValues {
		// cty.Value is immutable, so we don't need to copy these.
		localValues[k] = v
	}

	return &Module{
		Addr:         ms.Addr, // technically mutable, but immutable by convention
		Resources:    resources,
		OutputValues: outputValues,
		LocalValues:  localValues,
	}
}

// DeepCopy returns a new resource state that contains equivalent data to the
// receiver but shares no backing memory in common.
//
// As with all methods on Resource, this method is not safe to use concurrently
// with writing to any portion of the receiving data structure. It is the
// caller's responsibility to ensure mutual exclusion for the duration of the
// operation, but may then freely modify the receiver and the returned copy
// independently once this method returns.
func (rs *Resource) DeepCopy() *Resource {
	if rs == nil {
		return nil
	}

	instances := make(map[addrs.InstanceKey]*ResourceInstance, len(rs.Instances))
	for k, i := range rs.Instances {
		instances[k] = i.DeepCopy()
	}

	return &Resource{
		Addr:           rs.Addr,
		Instances:      instances,
		ProviderConfig: rs.ProviderConfig, // technically mutable, but immutable by convention
	}
}

// DeepCopy returns a new resource instance state that contains equivalent data
// to the receiver but shares no backing memory in common.
//
// As with all methods on ResourceInstance, this method is not safe to use
// concurrently with writing to any portion of the receiving data structure. It
// is the caller's responsibility to ensure mutual exclusion for the duration
// of the operation, but may then freely modify the receiver and the returned
// copy independently once this method returns.
func (i *ResourceInstance) DeepCopy() *ResourceInstance {
	if i == nil {
		return nil
	}

	deposed := make(map[DeposedKey]*ResourceInstanceObjectSrc, len(i.Deposed))
	for k, obj := range i.Deposed {
		deposed[k] = obj.DeepCopy()
	}

	return &ResourceInstance{
		Current:     i.Current.DeepCopy(),
		Deposed:     deposed,
		ProviderKey: i.ProviderKey,
	}
}

// DeepCopy returns a new resource instance object that contains equivalent data
// to the receiver but shares no backing memory in common.
//
// As with all methods on ResourceInstanceObjectSrc, this method is not safe to
// use concurrently with writing to any portion of the receiving data structure.
// It is the caller's responsibility to ensure mutual exclusion for the duration
// of the operation, but may then freely modify the receiver and the returned
// copy independently once this method returns.
func (os *ResourceInstanceObjectSrc) DeepCopy() *ResourceInstanceObjectSrc {
	if os == nil {
		return nil
	}

	var attrsFlat map[string]string
	if os.AttrsFlat != nil {
		attrsFlat = make(map[string]string, len(os.AttrsFlat))
		for k, v := range os.AttrsFlat {
			attrsFlat[k] = v
		}
	}

	var attrsJSON []byte
	if os.AttrsJSON != nil {
		attrsJSON = make([]byte, len(os.AttrsJSON))
		copy(attrsJSON, os.AttrsJSON)
	}

	var attrPaths []cty.PathValueMarks
	if os.AttrSensitivePaths != nil {
		attrPaths = make([]cty.PathValueMarks, len(os.AttrSensitivePaths))
		copy(attrPaths, os.AttrSensitivePaths)
	}

	var allAttrPaths []cty.PathValueMarks
	if os.TransientPathValueMarks != nil {
		allAttrPaths = make([]cty.PathValueMarks, len(os.TransientPathValueMarks))
		copy(allAttrPaths, os.TransientPathValueMarks)
	}

	var private []byte
	if os.Private != nil {
		private = make([]byte, len(os.Private))
		copy(private, os.Private)
	}

	// Some addrs.Referenceable implementations are technically mutable, but
	// we treat them as immutable by convention and so we don't deep-copy here.
	var dependencies []addrs.ConfigResource
	if os.Dependencies != nil {
		dependencies = make([]addrs.ConfigResource, len(os.Dependencies))
		copy(dependencies, os.Dependencies)
	}

	return &ResourceInstanceObjectSrc{
		Status:                  os.Status,
		SchemaVersion:           os.SchemaVersion,
		Private:                 private,
		AttrsFlat:               attrsFlat,
		AttrsJSON:               attrsJSON,
		AttrSensitivePaths:      attrPaths,
		TransientPathValueMarks: allAttrPaths,
		Dependencies:            dependencies,
		CreateBeforeDestroy:     os.CreateBeforeDestroy,
	}
}

// DeepCopy returns a new resource instance object that contains equivalent data
// to the receiver but shares no backing memory in common.
//
// As with all methods on ResourceInstanceObject, this method is not safe to use
// concurrently with writing to any portion of the receiving data structure. It
// is the caller's responsibility to ensure mutual exclusion for the duration
// of the operation, but may then freely modify the receiver and the returned
// copy independently once this method returns.
func (o *ResourceInstanceObject) DeepCopy() *ResourceInstanceObject {
	if o == nil {
		return nil
	}

	var private []byte
	if o.Private != nil {
		private = make([]byte, len(o.Private))
		copy(private, o.Private)
	}

	// Some addrs.Referenceable implementations are technically mutable, but
	// we treat them as immutable by convention and so we don't deep-copy here.
	var dependencies []addrs.ConfigResource
	if o.Dependencies != nil {
		dependencies = make([]addrs.ConfigResource, len(o.Dependencies))
		copy(dependencies, o.Dependencies)
	}

	return &ResourceInstanceObject{
		Value:               o.Value,
		Status:              o.Status,
		Private:             private,
		Dependencies:        dependencies,
		CreateBeforeDestroy: o.CreateBeforeDestroy,
	}
}

// DeepCopy returns a new output value state that contains equivalent data
// to the receiver but shares no backing memory in common.
//
// As with all methods on OutputValue, this method is not safe to use
// concurrently with writing to any portion of the receiving data structure. It
// is the caller's responsibility to ensure mutual exclusion for the duration
// of the operation, but may then freely modify the receiver and the returned
// copy independently once this method returns.
func (os *OutputValue) DeepCopy() *OutputValue {
	if os == nil {
		return nil
	}

	return &OutputValue{
		Addr:       os.Addr,
		Value:      os.Value,
		Sensitive:  os.Sensitive,
		Deprecated: os.Deprecated,
	}
}
