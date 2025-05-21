package cache

import (
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type Eval struct {
	resourcesLock sync.Mutex
	resources     map[string]*cacheEntry

	modulesLock sync.Mutex
	modules     map[string]*cacheEntry
}

func NewEval() *Eval {
	return &Eval{
		resources: map[string]*cacheEntry{},
		modules:   map[string]*cacheEntry{},
	}
}

type cacheEntry struct {
	sync.Mutex

	populated bool
	value     cty.Value
	diags     tfdiags.Diagnostics
}

func (e *Eval) Resource(addr addrs.AbsResource, populate func() (cty.Value, tfdiags.Diagnostics)) (cty.Value, tfdiags.Diagnostics) {
	key := addr.String()

	e.resourcesLock.Lock()
	entry, ok := e.resources[key]
	if !ok {
		entry = &cacheEntry{populated: false}
		e.resources[key] = entry
	}
	e.resourcesLock.Unlock()

	entry.Lock()
	defer entry.Unlock()
	if !entry.populated {
		entry.value, entry.diags = populate()
		entry.populated = true
	}

	return entry.value, entry.diags
}

func (e *Eval) EvictResource(addr addrs.AbsResource) {
	key := addr.String()

	e.resourcesLock.Lock()
	defer e.resourcesLock.Unlock()

	entry, ok := e.resources[key]
	if ok {
		entry.populated = false
	}
}

func (e *Eval) Module(addr addrs.AbsModuleCall, populate func() (cty.Value, tfdiags.Diagnostics)) (cty.Value, tfdiags.Diagnostics) {
	key := addr.String()

	e.modulesLock.Lock()
	entry, ok := e.modules[key]
	if !ok {
		entry = &cacheEntry{populated: false}
		e.modules[key] = entry
	}
	e.modulesLock.Unlock()

	entry.Lock()
	defer entry.Unlock()
	if !entry.populated {
		entry.value, entry.diags = populate()
		entry.populated = true
	}

	return entry.value, entry.diags
}

func (e *Eval) EvictModule(addr addrs.AbsModuleCall) {
	key := addr.String()

	e.modulesLock.Lock()
	defer e.modulesLock.Unlock()

	entry, ok := e.modules[key]
	if ok {
		entry.populated = false
	}
}
