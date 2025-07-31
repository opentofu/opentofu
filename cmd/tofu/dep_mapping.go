// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"path/filepath"

	"github.com/opentofu/opentofu/internal/depsrccfgs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

const dependencyMappingFilesEnv = "OPENTOFU_DEPENDENCY_MAPS"

func loadDependencyMappingConfigFiles(implicitStartDir string, envBaseDir string) ([]*depsrccfgs.Config, tfdiags.Diagnostics) {
	var filenames []string
	if raw := os.Getenv(dependencyMappingFilesEnv); raw != "" {
		filenames = filepath.SplitList(raw)
		// Relative paths in the environment must be resolved against
		// envBaseDir, rather than the current working directory.
		for i, filename := range filenames {
			if filepath.IsAbs(filename) {
				continue
			}
			filenames[i] = filepath.Join(envBaseDir, filename)
		}
	}
	// We use the implicit files _in addition to_ any explicitly-defined
	// ones when both are present, with the environment-based ones taking
	// priority so they can trample over rules in the implicit files if
	// needed. For example, automation might use this to force a different
	// installation approach for certain dependencies while still letting
	// the implicit rules take effect for anything the automation doesn't
	// care about.
	filenames = append(filenames, depsrccfgs.FindImplicitConfigFiles(implicitStartDir)...)

	if len(filenames) == 0 {
		return nil, nil
	}

	ret := make([]*depsrccfgs.Config, 0, len(filenames))
	var diags tfdiags.Diagnostics
	for _, filename := range filenames {
		config, moreDiags := depsrccfgs.LoadConfigFile(filename)
		diags = diags.Append(moreDiags)
		if moreDiags.HasErrors() {
			// Because the rules in earlier files are supposed to override
			// rules in later files, we don't include any subsequent files
			// after we fail to load one.
			// FIXME: It would be helpful to still try to load the remaining
			// ones to return any errors or warnings they produce even though
			// we aren't going to return them.
			break
		}
		ret = append(ret, config)
	}
	return ret, diags
}
