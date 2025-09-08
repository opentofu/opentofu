// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"io/fs"
	"path"
)

// FIXME: Everything in here should have unit tests

// ResolveRelativeModuleSource calculates a new source address from the
// combination of two other source addresses, if possible.
func ResolveRelativeModuleSource(a, b ModuleSource) (ModuleSource, error) {
	bLocal, ok := b.(ModuleSourceLocal)
	if !ok {
		return b, nil // non-local source addresses are always absolute
	}
	bRaw := string(bLocal)

	switch a := a.(type) {
	case ModuleSourceLocal:
		aRaw := string(a)
		new := path.Join(aRaw, bRaw)
		if !isModuleSourceLocal(new) {
			new = "./" + new // ModuleSourceLocal must always have a suitable prefix
		}
		return ModuleSourceLocal(new), nil
	case ModuleSourceRegistry:
		aSub := a.Subdir
		newSub, err := joinModuleSourceSubPath(aSub, bRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid relative path from %s: %w", a.String(), err)
		}
		return ModuleSourceRegistry{
			Package: a.Package,
			Subdir:  newSub,
		}, nil
	case ModuleSourceRemote:
		aSub := a.Subdir
		newSub, err := joinModuleSourceSubPath(aSub, bRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid relative path from %s: %w", a.String(), err)
		}
		return ModuleSourceRemote{
			Package: a.Package,
			Subdir:  newSub,
		}, nil
	default:
		// Should not get here, because the cases above should cover all
		// of the implementations of [ModuleSource].
		panic(fmt.Sprintf("unsupported ModuleSource type %T", a))
	}
}

func joinModuleSourceSubPath(subPath, rel string) (string, error) {
	new := path.Join(subPath, rel)
	if new == "." {
		return "", nil // the root of the package
	}
	// If subPath was valid then "." and ".." segments can only appear
	// if "rel" has too many "../" segments, causing it to "escape" from
	// its package root.
	if !fs.ValidPath(new) {
		return "", fmt.Errorf("relative path %s has too many \"../\" segments", rel)
	}
	return new, nil
}
