// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package funcs

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

func redactIfSensitiveOrEphemeral(value interface{}, valueMarks ...cty.ValueMarks) string {
	isEphemeral := marks.Has(cty.DynamicVal.WithMarks(valueMarks...), marks.Ephemeral)
	isSensitive := marks.Has(cty.DynamicVal.WithMarks(valueMarks...), marks.Sensitive)
	if isEphemeral && isSensitive {
		return "(ephemeral sensitive value)"
	}
	if isEphemeral {
		return "(ephemeral value)"
	}
	if isSensitive {
		return "(sensitive value)"
	}
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
