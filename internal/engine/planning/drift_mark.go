// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/ctymarks"
)

// driftMark represents the address and path within that address which has drifted.
// It abuses pointers to avoid serialization, with the caveat that the planning
// engine should only ever create one instance of driftMark[addr, path] to prevent
// unexpected duplicate marks.
type driftMark struct {
	addr *addrs.AbsResourceInstance
	path *cty.Path
}

func newDriftMark(addr addrs.AbsResourceInstance, path cty.Path) any {
	return driftMark{
		addr: &addr,
		path: new(path.Copy()),
	}
}

func withoutDriftMarks(v cty.Value) cty.Value {
	v, _ = v.WrangleMarksDeep(func(mark any, path cty.Path) (ctymarks.WrangleAction, error) {
		if _, isOurMark := mark.(driftMark); isOurMark {
			return ctymarks.WrangleDrop, nil
		}
		return nil, nil // leave all other marks alone
	})
	return v
}
