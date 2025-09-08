// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package instances

import (
	"fmt"
	"sort"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty"
)

// Expander instances serve as a coordination point for gathering object
// repetition values (count and for_each in configuration) and then later
// making use of them to fully enumerate all of the instances of an object.
//
// The two repeatable object types in OpenTofu are modules and resources.
// Because resources belong to modules and modules can nest inside other
// modules, module expansion in particular has a recursive effect that can
// cause deep objects to expand exponentially. Expander assumes that all
// instances of a module have the same static objects inside, and that they
// differ only in the repetition count for some of those objects.
//
// Expander is a synchronized object whose methods can be safely called
// from concurrent threads of execution. However, it does expect a certain
// sequence of operations which is normally obtained by the caller traversing
// a dependency graph: each object must have its repetition mode set exactly
// once, and this must be done before any calls that depend on the repetition
// mode. In other words, the count or for_each expression value for a module
// must be provided before any object nested directly or indirectly inside
// that module can be expanded. If this ordering is violated, the methods
// will panic to enforce internal consistency.
//
// The Expand* methods of Expander only work directly with modules and with
// resources. Addresses for other objects that nest within modules but
// do not themselves support repetition can be obtained by calling ExpandModule
// with the containing module path and then producing one absolute instance
// address per module instance address returned.
type Expander struct {
	mu   sync.RWMutex
	exps *expanderModule
}

// NewExpander initializes and returns a new Expander, empty and ready to use.
func NewExpander() *Expander {
	return &Expander{
		exps: newExpanderModule(),
	}
}

// SetModuleSingle records that the given module call inside the given parent
// module does not use any repetition arguments and is therefore a singleton.
func (e *Expander) SetModuleSingle(parentAddr addrs.ModuleInstance, callAddr addrs.ModuleCall) {
	e.setModuleExpansion(parentAddr, callAddr, expansionSingleVal)
}

// SetModuleCount records that the given module call inside the given parent
// module instance uses the "count" repetition argument, with the given value.
func (e *Expander) SetModuleCount(parentAddr addrs.ModuleInstance, callAddr addrs.ModuleCall, count int) {
	e.setModuleExpansion(parentAddr, callAddr, expansionCount(count))
}

// SetModuleForEach records that the given module call inside the given parent
// module instance uses the "for_each" repetition argument, with the given
// map value.
//
// In the configuration language the for_each argument can also accept a set.
// It's the caller's responsibility to convert that into an identity map before
// calling this method.
func (e *Expander) SetModuleForEach(parentAddr addrs.ModuleInstance, callAddr addrs.ModuleCall, mapping map[string]cty.Value) {
	e.setModuleExpansion(parentAddr, callAddr, expansionForEach(mapping))
}

// SetResourceSingle records that the given resource inside the given module
// does not use any repetition arguments and is therefore a singleton.
func (e *Expander) SetResourceSingle(moduleAddr addrs.ModuleInstance, resourceAddr addrs.Resource) {
	e.setResourceExpansion(moduleAddr, resourceAddr, expansionSingleVal)
}

// SetResourceCount records that the given resource inside the given module
// uses the "count" repetition argument, with the given value.
func (e *Expander) SetResourceCount(moduleAddr addrs.ModuleInstance, resourceAddr addrs.Resource, count int) {
	e.setResourceExpansion(moduleAddr, resourceAddr, expansionCount(count))
}

// SetResourceEnabled records that the given resource inside the given module
// uses the "enabled" repetition argument, with the given value.
func (e *Expander) SetResourceEnabled(moduleAddr addrs.ModuleInstance, resourceAddr addrs.Resource, enabled bool) {
	e.setResourceExpansion(moduleAddr, resourceAddr, expansionEnabled(enabled))
}

// SetResourceForEach records that the given resource inside the given module
// uses the "for_each" repetition argument, with the given map value.
//
// In the configuration language the for_each argument can also accept a set.
// It's the caller's responsibility to convert that into an identity map before
// calling this method.
func (e *Expander) SetResourceForEach(moduleAddr addrs.ModuleInstance, resourceAddr addrs.Resource, mapping map[string]cty.Value) {
	e.setResourceExpansion(moduleAddr, resourceAddr, expansionForEach(mapping))
}

// ExpandModule finds the exhaustive set of module instances resulting from
// the expansion of the given module and all of its ancestor modules.
//
// All of the modules on the path to the identified module must already have
// had their expansion registered using one of the SetModule* methods before
// calling, or this method will panic.
func (e *Expander) ExpandModule(addr addrs.Module) []addrs.ModuleInstance {
	return e.expandModule(addr, false)
}

// expandModule allows skipping unexpanded module addresses by setting skipUnknown to true.
// This is used by instances.Set, which is only concerned with the expanded
// instances, and should not panic when looking up unknown addresses.
func (e *Expander) expandModule(addr addrs.Module, skipUnknown bool) []addrs.ModuleInstance {
	if len(addr) == 0 {
		// Root module is always a singleton.
		return singletonRootModule
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// We're going to be dynamically growing ModuleInstance addresses, so
	// we'll preallocate some space to do it so that for typical shallow
	// module trees we won't need to reallocate this.
	// (moduleInstances does plenty of allocations itself, so the benefit of
	// pre-allocating this is marginal but it's not hard to do.)
	parentAddr := make(addrs.ModuleInstance, 0, 4)
	ret := e.exps.moduleInstances(addr, parentAddr, skipUnknown)
	sort.SliceStable(ret, func(i, j int) bool {
		return ret[i].Less(ret[j])
	})
	return ret
}

// GetDeepestExistingModuleInstance is a funny specialized function for
// determining how many steps we can traverse through the given module instance
// address before encountering an undeclared instance of a declared module.
//
// The result is the longest prefix of the given address which steps only
// through module instances that exist.
//
// All of the modules on the given path must already have had their
// expansion registered using one of the SetModule* methods before calling,
// or this method will panic.
func (e *Expander) GetDeepestExistingModuleInstance(given addrs.ModuleInstance) addrs.ModuleInstance {
	exps := e.exps // start with the root module expansions
	for i := 0; i < len(given); i++ {
		step := given[i]
		callName := step.Name
		if _, ok := exps.moduleCalls[addrs.ModuleCall{Name: callName}]; !ok {
			// This is a bug in the caller, because it should always register
			// expansions for an object and all of its ancestors before requesting
			// expansion of it.
			panic(fmt.Sprintf("no expansion has been registered for %s", given[:i].Child(callName, addrs.NoKey)))
		}

		var ok bool
		exps, ok = exps.childInstances[step]
		if !ok {
			// We've found a non-existing instance, so we're done.
			return given[:i]
		}
	}

	// If we complete the loop above without returning early then the entire
	// given address refers to a declared module instance.
	return given
}

// ExpandModuleResource finds the exhaustive set of resource instances resulting from
// the expansion of the given resource and all of its containing modules.
//
// All of the modules on the path to the identified resource and the resource
// itself must already have had their expansion registered using one of the
// SetModule*/SetResource* methods before calling, or this method will panic.
func (e *Expander) ExpandModuleResource(moduleAddr addrs.Module, resourceAddr addrs.Resource) []addrs.AbsResourceInstance {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// We're going to be dynamically growing ModuleInstance addresses, so
	// we'll preallocate some space to do it so that for typical shallow
	// module trees we won't need to reallocate this.
	// (moduleInstances does plenty of allocations itself, so the benefit of
	// pre-allocating this is marginal but it's not hard to do.)
	moduleInstanceAddr := make(addrs.ModuleInstance, 0, 4)
	ret := e.exps.moduleResourceInstances(moduleAddr, resourceAddr, moduleInstanceAddr)
	sort.SliceStable(ret, func(i, j int) bool {
		return ret[i].Less(ret[j])
	})
	return ret
}

// ExpandResource finds the set of resource instances resulting from
// the expansion of the given resource within its module instance.
//
// All of the modules on the path to the identified resource and the resource
// itself must already have had their expansion registered using one of the
// SetModule*/SetResource* methods before calling, or this method will panic.
//
// ExpandModuleResource returns all instances of a resource across all
// instances of its containing module, whereas this ExpandResource function
// is more specific and only expands within a single module instance. If
// any of the module instances selected in the module path of the given address
// aren't valid for that module's expansion then ExpandResource returns an
// empty result, reflecting that a non-existing module instance can never
// contain any existing resource instances.
func (e *Expander) ExpandResource(resourceAddr addrs.AbsResource) []addrs.AbsResourceInstance {
	e.mu.RLock()
	defer e.mu.RUnlock()

	moduleInstanceAddr := make(addrs.ModuleInstance, 0, 4)
	ret := e.exps.resourceInstances(resourceAddr.Module, resourceAddr.Resource, moduleInstanceAddr)
	sort.SliceStable(ret, func(i, j int) bool {
		return ret[i].Less(ret[j])
	})
	return ret
}

// GetModuleInstanceRepetitionData returns an object describing the values
// that should be available for each.key, each.value, and count.index within
// the call block for the given module instance.
func (e *Expander) GetModuleInstanceRepetitionData(addr addrs.ModuleInstance) RepetitionData {
	if len(addr) == 0 {
		// The root module is always a singleton, so it has no repetition data.
		return RepetitionData{}
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	parentMod := e.findModule(addr[:len(addr)-1])
	lastStep := addr[len(addr)-1]
	exp, ok := parentMod.moduleCalls[addrs.ModuleCall{Name: lastStep.Name}]
	if !ok {
		panic(fmt.Sprintf("no expansion has been registered for %s", addr))
	}
	return exp.repetitionData(lastStep.InstanceKey)
}

// GetResourceInstanceRepetitionData returns an object describing the values
// that should be available for each.key, each.value, and count.index within
// the definition block for the given resource instance.
func (e *Expander) GetResourceInstanceRepetitionData(addr addrs.AbsResourceInstance) RepetitionData {
	e.mu.RLock()
	defer e.mu.RUnlock()

	parentMod := e.findModule(addr.Module)
	exp, ok := parentMod.resources[addr.Resource.Resource]
	if !ok {
		panic(fmt.Sprintf("no expansion has been registered for %s", addr.ContainingResource()))
	}
	return exp.repetitionData(addr.Resource.Key)
}

// AllInstances returns a set of all of the module and resource instances known
// to the expander.
//
// It generally doesn't make sense to call this until everything has already
// been fully expanded by calling the SetModule* and SetResource* functions.
// After that, the returned set is a convenient small API only for querying
// whether particular instance addresses appeared as a result of those
// expansions.
func (e *Expander) AllInstances() Set {
	return Set{e}
}

func (e *Expander) findModule(moduleInstAddr addrs.ModuleInstance) *expanderModule {
	// We expect that all of the modules on the path to our module instance
	// should already have expansions registered.
	mod := e.exps
	for i, step := range moduleInstAddr {
		next, ok := mod.childInstances[step]
		if !ok {
			// Top-down ordering of registration is part of the contract of
			// Expander, so this is always indicative of a bug in the caller.
			panic(fmt.Sprintf("no expansion has been registered for ancestor module %s", moduleInstAddr[:i+1]))
		}
		mod = next
	}
	return mod
}

func (e *Expander) setModuleExpansion(parentAddr addrs.ModuleInstance, callAddr addrs.ModuleCall, exp expansion) {
	e.mu.Lock()
	defer e.mu.Unlock()

	mod := e.findModule(parentAddr)
	if _, exists := mod.moduleCalls[callAddr]; exists {
		panic(fmt.Sprintf("expansion already registered for %s", parentAddr.Child(callAddr.Name, addrs.NoKey)))
	}
	// We'll also pre-register the child instances so that later calls can
	// populate them as the caller traverses the configuration tree.
	for _, key := range exp.instanceKeys() {
		step := addrs.ModuleInstanceStep{Name: callAddr.Name, InstanceKey: key}
		mod.childInstances[step] = newExpanderModule()
	}
	mod.moduleCalls[callAddr] = exp
}

func (e *Expander) setResourceExpansion(parentAddr addrs.ModuleInstance, resourceAddr addrs.Resource, exp expansion) {
	e.mu.Lock()
	defer e.mu.Unlock()

	mod := e.findModule(parentAddr)
	if _, exists := mod.resources[resourceAddr]; exists {
		panic(fmt.Sprintf("expansion already registered for %s", resourceAddr.Absolute(parentAddr)))
	}
	mod.resources[resourceAddr] = exp
}

func (e *Expander) knowsModuleInstance(want addrs.ModuleInstance) bool {
	if want.IsRoot() {
		return true // root module instance is always present
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	return e.exps.knowsModuleInstance(want)
}

func (e *Expander) knowsModuleCall(want addrs.AbsModuleCall) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.exps.knowsModuleCall(want)
}

func (e *Expander) knowsResourceInstance(want addrs.AbsResourceInstance) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.exps.knowsResourceInstance(want)
}

func (e *Expander) knowsResource(want addrs.AbsResource) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.exps.knowsResource(want)
}

type expanderModule struct {
	moduleCalls    map[addrs.ModuleCall]expansion
	resources      map[addrs.Resource]expansion
	childInstances map[addrs.ModuleInstanceStep]*expanderModule
}

func newExpanderModule() *expanderModule {
	return &expanderModule{
		moduleCalls:    make(map[addrs.ModuleCall]expansion),
		resources:      make(map[addrs.Resource]expansion),
		childInstances: make(map[addrs.ModuleInstanceStep]*expanderModule),
	}
}

var singletonRootModule = []addrs.ModuleInstance{addrs.RootModuleInstance}

// if moduleInstances is being used to lookup known instances after all
// expansions have been done, set skipUnknown to true which allows addrs which
// may not have been seen to return with no instances rather than panicking.
func (m *expanderModule) moduleInstances(addr addrs.Module, parentAddr addrs.ModuleInstance, skipUnknown bool) []addrs.ModuleInstance {
	callName := addr[0]
	exp, ok := m.moduleCalls[addrs.ModuleCall{Name: callName}]
	if !ok {
		if skipUnknown {
			return nil
		}
		// This is a bug in the caller, because it should always register
		// expansions for an object and all of its ancestors before requesting
		// expansion of it.
		panic(fmt.Sprintf("no expansion has been registered for %s", parentAddr.Child(callName, addrs.NoKey)))
	}

	var ret []addrs.ModuleInstance

	// If there's more than one step remaining then we need to traverse deeper.
	if len(addr) > 1 {
		for step, inst := range m.childInstances {
			if step.Name != callName {
				continue
			}
			instAddr := append(parentAddr, step)
			ret = append(ret, inst.moduleInstances(addr[1:], instAddr, skipUnknown)...)
		}
		return ret
	}

	// Otherwise, we'll use the expansion from the final step to produce
	// a sequence of addresses under this prefix.
	for _, k := range exp.instanceKeys() {
		// We're reusing the buffer under parentAddr as we recurse through
		// the structure, so we need to copy it here to produce a final
		// immutable slice to return.
		full := make(addrs.ModuleInstance, 0, len(parentAddr)+1)
		full = append(full, parentAddr...)
		full = full.Child(callName, k)
		ret = append(ret, full)
	}
	return ret
}

func (m *expanderModule) moduleResourceInstances(moduleAddr addrs.Module, resourceAddr addrs.Resource, parentAddr addrs.ModuleInstance) []addrs.AbsResourceInstance {
	if len(moduleAddr) > 0 {
		var ret []addrs.AbsResourceInstance
		// We need to traverse through the module levels first, so we can
		// then iterate resource expansions in the context of each module
		// path leading to them.
		callName := moduleAddr[0]
		if _, ok := m.moduleCalls[addrs.ModuleCall{Name: callName}]; !ok {
			// This is a bug in the caller, because it should always register
			// expansions for an object and all of its ancestors before requesting
			// expansion of it.
			panic(fmt.Sprintf("no expansion has been registered for %s", parentAddr.Child(callName, addrs.NoKey)))
		}

		for step, inst := range m.childInstances {
			if step.Name != callName {
				continue
			}
			moduleInstAddr := append(parentAddr, step)
			ret = append(ret, inst.moduleResourceInstances(moduleAddr[1:], resourceAddr, moduleInstAddr)...)
		}
		return ret
	}

	return m.onlyResourceInstances(resourceAddr, parentAddr)
}

func (m *expanderModule) resourceInstances(moduleAddr addrs.ModuleInstance, resourceAddr addrs.Resource, parentAddr addrs.ModuleInstance) []addrs.AbsResourceInstance {
	if len(moduleAddr) > 0 {
		// We need to traverse through the module levels first, using only the
		// module instances for our specific resource, as the resource may not
		// yet be expanded in all module instances.
		step := moduleAddr[0]
		callName := step.Name
		if _, ok := m.moduleCalls[addrs.ModuleCall{Name: callName}]; !ok {
			// This is a bug in the caller, because it should always register
			// expansions for an object and all of its ancestors before requesting
			// expansion of it.
			panic(fmt.Sprintf("no expansion has been registered for %s", parentAddr.Child(callName, addrs.NoKey)))
		}

		if inst, ok := m.childInstances[step]; ok {
			moduleInstAddr := append(parentAddr, step)
			return inst.resourceInstances(moduleAddr[1:], resourceAddr, moduleInstAddr)
		} else {
			// If we have the module _call_ registered (as we checked above)
			// but we don't have the given module _instance_ registered, that
			// suggests that the module instance key in "step" is not declared
			// by the current definition of this module call. That means the
			// module instance doesn't exist at all, and therefore it can't
			// possibly declare any resource instances either.
			//
			// For example, if we were asked about module.foo[0].aws_instance.bar
			// but module.foo doesn't currently have count set, then there is no
			// module.foo[0] at all, and therefore no aws_instance.bar
			// instances inside it.
			return nil
		}
	}
	return m.onlyResourceInstances(resourceAddr, parentAddr)
}

func (m *expanderModule) onlyResourceInstances(resourceAddr addrs.Resource, parentAddr addrs.ModuleInstance) []addrs.AbsResourceInstance {
	var ret []addrs.AbsResourceInstance
	exp, ok := m.resources[resourceAddr]
	if !ok {
		panic(fmt.Sprintf("no expansion has been registered for %s", resourceAddr.Absolute(parentAddr)))
	}

	for _, k := range exp.instanceKeys() {
		// We're reusing the buffer under parentAddr as we recurse through
		// the structure, so we need to copy it here to produce a final
		// immutable slice to return.
		moduleAddr := make(addrs.ModuleInstance, len(parentAddr))
		copy(moduleAddr, parentAddr)
		ret = append(ret, resourceAddr.Instance(k).Absolute(moduleAddr))
	}
	return ret
}

func (m *expanderModule) getModuleInstance(want addrs.ModuleInstance) *expanderModule {
	current := m
	for _, step := range want {
		next := current.childInstances[step]
		if next == nil {
			return nil
		}
		current = next
	}
	return current
}

func (m *expanderModule) knowsModuleInstance(want addrs.ModuleInstance) bool {
	return m.getModuleInstance(want) != nil
}

func (m *expanderModule) knowsModuleCall(want addrs.AbsModuleCall) bool {
	modInst := m.getModuleInstance(want.Module)
	if modInst == nil {
		return false
	}
	_, ret := modInst.moduleCalls[want.Call]
	return ret
}

func (m *expanderModule) knowsResourceInstance(want addrs.AbsResourceInstance) bool {
	modInst := m.getModuleInstance(want.Module)
	if modInst == nil {
		return false
	}
	resourceExp := modInst.resources[want.Resource.Resource]
	if resourceExp == nil {
		return false
	}
	for _, key := range resourceExp.instanceKeys() {
		if key == want.Resource.Key {
			return true
		}
	}
	return false
}

func (m *expanderModule) knowsResource(want addrs.AbsResource) bool {
	modInst := m.getModuleInstance(want.Module)
	if modInst == nil {
		return false
	}
	_, ret := modInst.resources[want.Resource]
	return ret
}
