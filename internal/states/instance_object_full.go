// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package states

import (
	"slices"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/marks"
)

// The definitions in this file are currently for use only by the new runtime
// implementation in its "walking skeleton" phase. All code outside of that
// work-in-progress should continue to use [ResourceInstanceObject] and
// [ResourceInstanceObjectSrc], and their associated definitions in
// instance_object.go and instance_object_src.go .
//
// If at some later point we remove or rename the [ResourceInstanceObject] and
// [ResourceInstanceObjectSrc] types then the "Full"-suffixed symbol names here
// should probably also be renamed to drop that suffix, but we're waiting to
// do that until we're more sure that this is how we're going to represent
// resource instance objects in the new runtime.

// ResourceInstanceObjectFull is a variant of [ResourceInstanceObject] that
// incorporates all of the context from parent containers so that a pointer
// to an object of this type is sufficient to represent everything about
// a resource instance object without cross-referencing with a containing
// [State], [Module], [ResourceInstance], etc.
//
// Use [EncodeResourceInstanceObjectFull] with the appropriate schema from the
// appropriate provider to transform this into [ResourceInstanceObjectFullSrc].
type ResourceInstanceObjectFull = resourceInstanceObjectRepr[cty.Value]

// ResourceInstanceObjectFullSrc is to [ResourceInstanceObjectSrc] what
// [ResourceInstanceObjectFull] is to [ResourceInstanceObject]: a variant
// representation with the main value not yet decoded, so that the caller
// can delay unmarshaling until it's able to obtain the necessary schema
// from the provider.
//
// Use [DecodeResourceInstanceObjectFull] with the appropriate schema from the
// appropriate provider to transform this into [ResourceInstanceObjectFull].
type ResourceInstanceObjectFullSrc = resourceInstanceObjectRepr[ValueJSONWithMetadata]

func DecodeResourceInstanceObjectFull(src *ResourceInstanceObjectFullSrc, ty cty.Type) (*ResourceInstanceObjectFull, error) {
	v, err := src.Value.Decode(ty)
	if err != nil {
		return nil, err
	}
	return mapResourceInstanceObjectReprValue(src, v), nil
}

func EncodeResourceInstanceObjectFull(obj *ResourceInstanceObjectFull, ty cty.Type) (*ResourceInstanceObjectFullSrc, error) {
	vSrc, err := EncodeValueJSONWithMetadata(obj.Value, ty)
	if err != nil {
		return nil, err
	}
	return mapResourceInstanceObjectReprValue(obj, vSrc), nil
}

// ResourceInstanceObjectFull returns a snapshot of the instance object of
// the given instance address and deposed key, or nil if no such object is
// tracked.
//
// Set deposedKey to [NotDeposed] to retrieve the "current" object associated
// with the given resource instance address, if any.
//
// This is currently for use with the experimental new language runtime only.
// Callers from the old runtime should use [SyncState.ResourceInstanceObject]
// instead. The "full" form is extended so that the returned object is a
// self-sufficient description of everything we store that's relevant to the
// requested resource instance object, without the recipient needing to refer
// to any other part of the state data structure.
//
// The return value is a pointer to a copy of the object, which the caller
// may then freely access and mutate.
func (s *SyncState) ResourceInstanceObjectFull(addr addrs.AbsResourceInstance, deposedKey DeposedKey) *ResourceInstanceObjectFullSrc {
	s.lock.RLock()
	defer s.lock.RUnlock()

	rsrc := s.state.Resource(addr.ContainingResource())
	if rsrc == nil {
		return nil
	}
	inst := rsrc.Instances[addr.Resource.Key]
	if inst == nil {
		return nil
	}
	var srcObj *ResourceInstanceObjectSrc
	if deposedKey == NotDeposed {
		srcObj = inst.Current
	} else {
		srcObj = inst.Deposed[deposedKey]
	}
	if srcObj == nil {
		return nil
	}

	// We need to shim from the disjointed representation to the unified
	// form of provider instance address that the new runtime prefers.
	providerInstAddr := addrs.AbsProviderInstanceCorrect{
		Config: rsrc.ProviderConfig.Correct(),
		Key:    inst.ProviderKey,
	}

	// For now our "deep copy" will take the form of explicitly translating the
	// [ResourceInstanceObjectSrc] representation into the equivalent
	// [ResourceInstanceObjectFullSrc] representation. We'll need to do
	// something different here in future if we remove the old-shaped
	// representation and use the "full" representation as the primary one.
	return &ResourceInstanceObjectFullSrc{
		Value: ValueJSONWithMetadata{
			// We're assuming here that we'll never encounter a legacy state
			// snapshot that uses AttrsFlat, because that form hasn't been
			// used since Terraform v0.11 and we don't support upgrading
			// to OpenTofu directly from such an old version of Terraform.
			ValueJSON: slices.Clone(srcObj.AttrsJSON),
			SensitivePaths: slices.Collect(func(yield func(cty.Path) bool) {
				for _, pvm := range srcObj.AttrSensitivePaths {
					if !yield(pvm.Path) {
						return
					}
				}
			}),
		},
		Private:              slices.Clone(srcObj.Private),
		Status:               srcObj.Status,
		ProviderInstanceAddr: providerInstAddr,
		ResourceType:         addr.Resource.Resource.Type,
		SchemaVersion:        srcObj.SchemaVersion,
		Dependencies:         slices.Clone(srcObj.Dependencies),
		CreateBeforeDestroy:  srcObj.CreateBeforeDestroy,
	}
}

// SetResourceInstanceObjectFull stores a new state for the specified resource
// instance object, overwriting an existing object with the same identity
// if present.
//
// Set deposedKey to [NotDeposed] to set the "current" object associated
// with the given resource instance address.
//
// This is currently for use with the experimental new language runtime only.
// Callers from the old runtime should use [SyncState.SetResourceInstance]
// or similar instead. The "full" form is extended so that the given object is a
// self-sufficient description of everything we store that's relevant to the
// requested resource instance object.
//
// Note that our current state model cannot support different objects for
// a single resource having different provider configuration addresses, or
// different resource instances of the same resource having different provider
// instance keys, and so currently this will clobber the provider configuration
// address for other objects in the same scope if they differ. In practice our
// language doesn't permit them to differ at the time of writing anyway and
// so that's not a big deal, but we will probably want to update the state
// model at some point to remove this constraint that isn't actually necessary
// for the new language runtime.
func (s *SyncState) SetResourceInstanceObjectFull(addr addrs.AbsResourceInstance, deposedKey DeposedKey, obj *ResourceInstanceObjectFullSrc) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Currently this is a wrapper around various other methods as we
	// shim the new-style representation to fit the traditional representation.
	ms := s.state.EnsureModule(addr.Module)
	providerConfigAddr := addrs.AbsProviderConfig{
		// NOTE: This is currently a little lossy because
		// [addrs.AbsProviderConfig] is constrained by the limitations of our
		// old language runtime. In particular, it loses any instance keys
		// of modules in the module address, because the old runtime did not
		// permit provider configurations inside multi-instanced modules.
		// FIXME: Update our underlying model to support this more generally,
		// once we're confident enough about the new runtime to risk changes
		// that could impact code from the old runtime.
		Module:   obj.ProviderInstanceAddr.Config.Module.Module(),
		Provider: obj.ProviderInstanceAddr.Config.Config.Provider,
		Alias:    obj.ProviderInstanceAddr.Config.Config.Alias,
	}
	smallerObj := &ResourceInstanceObjectSrc{
		AttrsJSON:           obj.Value.ValueJSON,
		SchemaVersion:       obj.SchemaVersion,
		Status:              obj.Status,
		Private:             obj.Private,
		Dependencies:        obj.Dependencies,
		CreateBeforeDestroy: obj.CreateBeforeDestroy,
	}
	if len(obj.Value.SensitivePaths) != 0 {
		smallerObj.AttrSensitivePaths = make([]cty.PathValueMarks, len(obj.Value.SensitivePaths))
		marks := cty.NewValueMarks(marks.Sensitive)
		for i, path := range obj.Value.SensitivePaths {
			smallerObj.AttrSensitivePaths[i] = cty.PathValueMarks{
				Path:  path,
				Marks: marks,
			}
		}
	}
	if deposedKey == NotDeposed {
		ms.SetResourceInstanceCurrent(addr.Resource, smallerObj, providerConfigAddr, obj.ProviderInstanceAddr.Key)
	} else {
		ms.SetResourceInstanceDeposed(addr.Resource, deposedKey, smallerObj, providerConfigAddr, obj.ProviderInstanceAddr.Key)
	}
	s.maybePruneModule(addr.Module)
}

// ResourceInstanceObjectFullRepr is the generic type that both
// [ResourceInstanceObjectFull] and [ResourceInstanceObjectFullSrc] are based
// on, since they vary only by the type of the Value field.
type resourceInstanceObjectRepr[V interface {
	cty.Value | ValueJSONWithMetadata
}] struct {
	// Value is the object-typed value representing the remote object within
	// OpenTofu.
	Value V

	// Private is an opaque value set by the provider when this object was
	// last created or updated. OpenTofu Core does not use this value in
	// any way and it is not exposed anywhere in the user interface, so
	// a provider can use it for retaining any necessary private state.
	Private []byte

	// Status represents the "readiness" of the object as of the last time
	// it was updated.
	Status ObjectStatus

	// ProviderInstanceAddr is the fully-qualified address for the provider
	// instance that produced the data in the Value and Private fields, and
	// so which should be used to refresh and destroy this object if there's
	// no provider instance selection still present in the configuration.
	ProviderInstanceAddr addrs.AbsProviderInstanceCorrect

	// ResourceType is the resource type name as would be understood by the
	// provider given in ProviderInstanceAddr. This is captured explicitly
	// here, rather than just implied by context, so that when we're
	// dealing with an implicit conversion from one resource type to another
	// we can distinguish whether we've already performed that conversion
	// yet or not, and directly compare before and after objects to determine
	// whether such a conversion has occurred.
	ResourceType string

	// SchemaVersion is the resource-type-specific schema version number that
	// was current when either AttrsJSON or AttrsFlat was encoded. Migration
	// steps are required if this is less than the current version number
	// reported by the corresponding provider.
	SchemaVersion uint64

	// Dependencies is a set of absolute address to other resources this
	// instance depended on when it was applied. This is used to construct
	// the dependency relationships for an object whose configuration is no
	// longer available, such as if it has been removed from configuration
	// altogether, or is now deposed.
	//
	// FIXME: In the long run under the new runtime this should probably be
	// []addrs.AbsResourceInstance instead because the new runtime can track
	// dependencies more precisely, but this is using ConfigResource for now
	// just because that needs less shimming from the current underlying
	// representation, and so we can wait until we better understand what the
	// caller needs before we spend time implementing that.
	Dependencies []addrs.ConfigResource

	// CreateBeforeDestroy reflects the status of the lifecycle
	// create_before_destroy option when this instance was last updated.
	// Because create_before_destroy also effects the overall ordering of the
	// destroy operations, we need to record the status to ensure a resource
	// removed from the config will still be destroyed in the same manner.
	CreateBeforeDestroy bool
}

func mapResourceInstanceObjectReprValue[V1, V2 interface {
	cty.Value | ValueJSONWithMetadata
}](input *resourceInstanceObjectRepr[V1], newValue V2) *resourceInstanceObjectRepr[V2] {
	return &resourceInstanceObjectRepr[V2]{
		Value:                newValue,
		Private:              input.Private,
		Status:               input.Status,
		ProviderInstanceAddr: input.ProviderInstanceAddr,
		ResourceType:         input.ResourceType,
		SchemaVersion:        input.SchemaVersion,
		Dependencies:         input.Dependencies,
		CreateBeforeDestroy:  input.CreateBeforeDestroy,
	}
}

type ValueJSONWithMetadata struct {
	// ValueJSON is a JSON representation of the cty value.
	ValueJSON []byte

	// SensitivePaths is an array of paths to mark as sensitive when decoding.
	SensitivePaths []cty.Path
}

func (vj ValueJSONWithMetadata) Decode(ty cty.Type) (cty.Value, error) {
	unmarkedV, err := ctyjson.Unmarshal(vj.ValueJSON, ty)
	if err != nil {
		return cty.NilVal, err
	}
	if len(vj.SensitivePaths) == 0 {
		// If we don't have any marks to apply then we can skip a recursive
		// walk dealing with those.
		return unmarkedV, nil
	}
	// The following is O(Npaths*Nmarks), but Npaths is usually small for
	// typical resource types, and Nmarks should never be greater than Npaths
	// in a canonical representation (since paths that don't exist cannot
	// be marked), so it isn't worth the cost of trying to do something
	// cleverer.
	return cty.Transform(unmarkedV, func(path cty.Path, pathV cty.Value) (cty.Value, error) {
		for _, sensitivePath := range vj.SensitivePaths {
			if sensitivePath.Equals(path) {
				return pathV.Mark(marks.Sensitive), nil
			}
		}
		return pathV, nil
	})
}

func EncodeValueJSONWithMetadata(v cty.Value, ty cty.Type) (ValueJSONWithMetadata, error) {
	var ret ValueJSONWithMetadata
	unmarkedV, pvms := v.UnmarkDeepWithPaths()
	src, err := ctyjson.Marshal(unmarkedV, ty)
	if err != nil {
		return ret, err
	}
	ret.ValueJSON = src
	if len(pvms) != 0 {
		ret.SensitivePaths = make([]cty.Path, 0, len(pvms))
		for _, pvm := range pvms {
			for mark := range pvm.Marks {
				if mark != marks.Sensitive {
					// The caller is expected to strip out any other marks and
					// handle them in some reasonable way before calling this
					// function, to make sure that we don't silently lose
					// unexpected marks during marshaling.
					return ret, pvm.Path.NewErrorf("cannot encode value with mark %#v", mark)
				}
				ret.SensitivePaths = append(ret.SensitivePaths, pvm.Path)
			}
		}
	}
	return ret, nil
}
