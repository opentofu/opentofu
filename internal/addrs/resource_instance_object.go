// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"math/rand"
	"time"
)

// AbsResourceInstanceObject represents a single object associated with a
// resource instance.
//
// A resource instance has zero or one "current" objects and zero or more
// "deposed" objects. During a "create before destroy" replace operation the
// current object is transformed into a deposed object before creating the
// new replacement object, and then is destroyed only once the new current
// object has been successfully created.
type AbsResourceInstanceObject struct {
	// InstanceAddr is the address of the resource instance that this object
	// belongs to.
	InstanceAddr AbsResourceInstance

	// DeposedKey is the deposed key of a deposed object, or [NotDeposed] when
	// representing the "current" object for a resource instance.
	DeposedKey DeposedKey
}

// CurrentObject returns an [AbsResourceInstanceObject] representing the current
// object belonging to this resource instance.
func (ri AbsResourceInstance) CurrentObject() AbsResourceInstanceObject {
	return ri.Object(NotDeposed)
}

// CurrentObject returns an [AbsResourceInstanceObject] for this resource
// instance, using the given [DeposedKey].
//
// It's okay to pass [NotDeposed], but if you'd be passing that as a constant
// rather than as a calculated value then [AbsResourceInstance.CurrentObject]
// is a clearer way to communicate that idea.
func (ri AbsResourceInstance) Object(key DeposedKey) AbsResourceInstanceObject {
	return AbsResourceInstanceObject{
		InstanceAddr: ri,
		DeposedKey:   key,
	}
}

// IsCurrent returns true if this represents the "current" object of some
// resource instance.
func (o AbsResourceInstanceObject) IsCurrent() bool {
	return o.DeposedKey == NotDeposed
}

// IsDeposed returns true if this represents a "deposed" object of some
// resource instance.
func (o AbsResourceInstanceObject) IsDeposed() bool {
	return o.DeposedKey != NotDeposed
}

func (o AbsResourceInstanceObject) String() string {
	if o.DeposedKey == NotDeposed {
		return o.InstanceAddr.String()
	}
	return fmt.Sprintf("%s deposed object %q", o.InstanceAddr.String(), o.DeposedKey.String())
}

func (o AbsResourceInstanceObject) Equal(other AbsResourceInstanceObject) bool {
	return o.DeposedKey == other.DeposedKey && o.InstanceAddr.Equal(other.InstanceAddr)
}

func (o AbsResourceInstanceObject) Less(other AbsResourceInstanceObject) bool {
	if o.InstanceAddr.Less(other.InstanceAddr) {
		return true
	}
	if o.DeposedKey < other.DeposedKey {
		return true
	}
	return false
}

func (o AbsResourceInstanceObject) UniqueKey() UniqueKey {
	return absResourceInstanceObjectKey{
		InstanceAddr: o.InstanceAddr.String(),
		DeposedKey:   o.DeposedKey,
	}
}

// absResourceInstanceObjectKey is the [UniqueKey] type for
// [AbsResourceInstanceObject].
type absResourceInstanceObjectKey struct {
	InstanceAddr string
	DeposedKey   DeposedKey
}

// uniqueKeySigil implements [UniqueKey].
func (a absResourceInstanceObjectKey) uniqueKeySigil() {}

// DeposedKey is a 8-character hex string used to uniquely identify deposed
// instance objects in the state.
type DeposedKey string

// NotDeposed is a special value of [DeposedKey] used to represent the absense
// of a deposed key.
const NotDeposed = DeposedKey("")

var deposedKeyRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// NewDeposedKey generates a pseudo-random deposed key. Because of the short
// length of these keys, uniqueness is not a natural consequence and so the
// caller should test to see if the generated key is already in use and generate
// another if so, until a unique key is found.
func NewDeposedKey() DeposedKey {
	v := deposedKeyRand.Uint32()
	return DeposedKey(fmt.Sprintf("%08x", v))
}

func (k DeposedKey) String() string {
	return string(k)
}

func (k DeposedKey) GoString() string {
	ks := string(k)
	switch ks {
	case "":
		return "addrs.NotDeposed"
	default:
		return fmt.Sprintf("addrs.DeposedKey(%s)", ks)
	}
}

// Generation converts a [DeposedKey] into the legacy old representation
// [ResourceInstanceObjectGeneration].
//
// The older type comes from a time where things were modeled in a more
// complicated way, but in today's OpenTofu it's just a more expensive way to
// represent the same information that [DeposedKey] can represent, and so it
// should not be used in new code. Instead, just use [DeposedKey] directly and
// use [NotDeposed] to represent the "current" object for a resource instance.
func (k DeposedKey) Generation() ResourceInstanceObjectGeneration {
	// Our main requirement here is that we never return [NotDeposed] directly
	// as a real [ResourceInstanceObjectGeneration] value, because legacy code
	// expects "not deposed" to be represented as the zero value of this type,
	// which is a nil interface value.
	if k == NotDeposed {
		return CurrentResourceInstanceObjectGeneration
	}
	return k
}

// generation implements [ResourceInstanceObjectGeneration].
func (k DeposedKey) legacyResourceInstanceGeneration() {}

// ResourceInstanceObjectGeneration is a legacy type used to represent multiple
// objects in a succession of objects represented by a single resource instance
// address. Don't use this type in new code.
//
// The modern way to represent this information is to use a value of type
// [DeposedKey] and then set it to [NotDeposed] when representing a
// "current" object associated with a resource instance. We have this type
// here only because existing callers still refer to it through its alias in
// "package states", and all implementations of it are required to be in the
// same package and [DeposedKey] is one of the implementations.
//
// A Generation value can either be the value of the variable "CurrentGen" or
// a value of type DeposedKey. Generation values can be compared for equality
// using "==" and used as map keys.
type ResourceInstanceObjectGeneration interface {
	legacyResourceInstanceGeneration()
}

// CurrentResourceInstanceObjectGeneration is the zero value of
// [ResourceInstanceObjectGeneration], representing the currently-active object
// for a resource instance.
var CurrentResourceInstanceObjectGeneration ResourceInstanceObjectGeneration
