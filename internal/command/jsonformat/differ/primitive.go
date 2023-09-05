// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package differ

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentffoundation/opentf/internal/command/jsonformat/computed"
	"github.com/opentffoundation/opentf/internal/command/jsonformat/computed/renderers"
	"github.com/opentffoundation/opentf/internal/command/jsonformat/structured"
)

func computeAttributeDiffAsPrimitive(change structured.Change, ctype cty.Type) computed.Diff {
	return asDiff(change, renderers.Primitive(change.Before, change.After, ctype))
}
