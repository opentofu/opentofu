// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"

	version "github.com/hashicorp/go-version"
	"github.com/mitchellh/cli"
	"github.com/terramate-io/opentofulib/internal/initwd"
)

type uiModuleInstallHooks struct {
	initwd.ModuleInstallHooksImpl
	Ui             cli.Ui
	ShowLocalPaths bool
}

var _ initwd.ModuleInstallHooks = uiModuleInstallHooks{}

func (h uiModuleInstallHooks) Download(modulePath, packageAddr string, v *version.Version) {
	if v != nil {
		h.Ui.Info(fmt.Sprintf("Downloading %s %s for %s...", packageAddr, v, modulePath))
	} else {
		h.Ui.Info(fmt.Sprintf("Downloading %s for %s...", packageAddr, modulePath))
	}
}

func (h uiModuleInstallHooks) Install(modulePath string, v *version.Version, localDir string) {
	if h.ShowLocalPaths {
		h.Ui.Info(fmt.Sprintf("- %s in %s", modulePath, localDir))
	} else {
		h.Ui.Info(fmt.Sprintf("- %s", modulePath))
	}
}
