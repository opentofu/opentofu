// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package workdir

import (
	"fmt"
	"os"
)

func runChdirDirect(overrideWd string) error {
	if overrideWd != "" {
		err := os.Chdir(overrideWd)
		if err != nil {
			return fmt.Errorf("Error handling -chdir option: %s", err)
		}
	}
	return nil
}
