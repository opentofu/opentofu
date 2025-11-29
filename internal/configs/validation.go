// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
)

func complainRngAndMsg(countRng, enabledRng, forEachRng hcl.Range) (*hcl.Range, string) {
	var complainRngs []hcl.Range
	var complainAttrs []string
	if !countRng.Empty() {
		complainRngs = append(complainRngs, countRng)
		complainAttrs = append(complainAttrs, "\"count\"")
	}
	if !enabledRng.Empty() {
		complainRngs = append(complainRngs, enabledRng)
		complainAttrs = append(complainAttrs, "\"enabled\"")
	}
	if !forEachRng.Empty() {
		complainRngs = append(complainRngs, forEachRng)
		complainAttrs = append(complainAttrs, "\"for_each\"")
	}

	if len(complainAttrs) < 2 {
		// If there are less than two valid ranges, we return an empty range and an empty string
		return nil, ""
	}

	// We sort the complain ranges to return the range between the first and last valid range
	sort.SliceStable(complainRngs, func(i, j int) bool {
		return complainRngs[i].Start.Byte < complainRngs[j].Start.Byte
	})

	var sep string
	lastIndex := len(complainAttrs) - 1
	if len(complainAttrs) >= 3 {
		// Add an oxford comma to the last attribute
		complainAttrs[lastIndex] = "and " + complainAttrs[lastIndex]
		sep = ", "
	} else {
		sep = " and "
	}
	complainMsg := strings.Join(complainAttrs, sep)
	complainRng := hcl.RangeBetween(complainRngs[0], complainRngs[lastIndex])

	return complainRng.Ptr(), complainMsg
}
