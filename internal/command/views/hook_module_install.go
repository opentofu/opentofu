// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/opentofu/opentofu/internal/initwd"
)

// moduleInstallationHookHuman is the implementation of [initwd.ModuleInstallHooks] that prints the modules
// installation progress information in human readable format.
type moduleInstallationHookHuman struct {
	v              *View
	showLocalPaths bool
}

var _ initwd.ModuleInstallHooks = moduleInstallationHookHuman{}

func (h moduleInstallationHookHuman) Download(modulePath, packageAddr string, v *version.Version) {
	if v != nil {
		_, _ = h.v.streams.Println(fmt.Sprintf("Downloading %s %s for %s...", packageAddr, v, modulePath))
	} else {
		_, _ = h.v.streams.Println(fmt.Sprintf("Downloading %s for %s...", packageAddr, modulePath))
	}
}

func (h moduleInstallationHookHuman) Install(modulePath string, v *version.Version, localDir string) {
	if h.showLocalPaths {
		_, _ = h.v.streams.Println(fmt.Sprintf("- %s in %s", modulePath, localDir))
	} else {
		_, _ = h.v.streams.Println(fmt.Sprintf("- %s", modulePath))
	}
}

// moduleInstallationHookJSON is the implementation of [initwd.ModuleInstallHooks] that prints the modules
// installation progress information in JSON format.
type moduleInstallationHookJSON struct {
	v              *JSONView
	showLocalPaths bool
}

var _ initwd.ModuleInstallHooks = moduleInstallationHookJSON{}

func (h moduleInstallationHookJSON) Download(modulePath, packageAddr string, v *version.Version) {
	if v != nil {
		h.v.Info(fmt.Sprintf("Downloading %s %s for %s...", packageAddr, v, modulePath))
	} else {
		h.v.Info(fmt.Sprintf("Downloading %s for %s...", packageAddr, modulePath))
	}
}

func (h moduleInstallationHookJSON) Install(modulePath string, _ *version.Version, localDir string) {
	if h.showLocalPaths {
		h.v.Info(fmt.Sprintf("installing %s in %s", modulePath, localDir))
	} else {
		h.v.Info(fmt.Sprintf("installing %s", modulePath))
	}
}

// moduleInstallationHookMulti is the implementation of [initwd.ModuleInstallHooks] that wraps multiple
// implementation of [initwd.ModuleInstallHooks] and acts as a proxy for all of those.
// This is used for the `-json-into` flag.
type moduleInstallationHookMulti []initwd.ModuleInstallHooks

var _ initwd.ModuleInstallHooks = moduleInstallationHookMulti(nil)

func (m moduleInstallationHookMulti) Download(modulePath, packageAddr string, v *version.Version) {
	for _, h := range m {
		h.Download(modulePath, packageAddr, v)
	}
}

func (m moduleInstallationHookMulti) Install(modulePath string, v *version.Version, localDir string) {
	for _, h := range m {
		h.Install(modulePath, v, localDir)
	}
}
