// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ProvidersMirror interface {
	Diagnostics(diags tfdiags.Diagnostics)
	ProviderSkipped(provider string)
	MirroringProvider(provider string)
	ProviderVersionSelectedToMatchLockfile(provider string, version string)
	ProviderVersionSelectedToMatchConstraints(provider string, version string, constraints string)
	ProviderVersionSelectedWithNoConstraints(provider string, version string)
	DownloadingPackageFor(provider string, version string, platform string)
	PackageAuthenticated(provider string, version string, platform string, authResult string)
}

// NewProvidersMirror returns an initialized ProvidersMirror implementation for the given ViewType.
func NewProvidersMirror(args arguments.ViewOptions, view *View) ProvidersMirror {
	var ret ProvidersMirror
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &ProvidersMirrorJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &ProvidersMirrorHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &ProvidersMirrorMulti{ret, &ProvidersMirrorJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

type ProvidersMirrorHuman struct {
	view *View
}

var _ ProvidersMirror = (*ProvidersMirrorHuman)(nil)

func (v *ProvidersMirrorHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ProvidersMirrorHuman) ProviderSkipped(provider string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- Skipping %s because it is built in to OpenTofu CLI", provider))
}

func (v *ProvidersMirrorHuman) MirroringProvider(provider string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- Mirroring %s...", provider))
}

func (v *ProvidersMirrorHuman) ProviderVersionSelectedToMatchLockfile(_ string, version string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("  - Selected v%s to match dependency lock file", version))
}

func (v *ProvidersMirrorHuman) ProviderVersionSelectedToMatchConstraints(_ string, version string, constraints string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("  - Selected v%s to meet constraints %s", version, constraints))
}

func (v *ProvidersMirrorHuman) ProviderVersionSelectedWithNoConstraints(_ string, version string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("  - Selected v%s with no constraints", version))
}

func (v *ProvidersMirrorHuman) DownloadingPackageFor(_ string, _ string, platform string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("  - Downloading package for %s...", platform))
}

func (v *ProvidersMirrorHuman) PackageAuthenticated(_, _, _ string, authResult string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("  - Package authenticated: %s", authResult))
}

type ProvidersMirrorMulti []ProvidersMirror

var _ ProvidersMirror = (ProvidersMirrorMulti)(nil)

func (m ProvidersMirrorMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m ProvidersMirrorMulti) ProviderSkipped(provider string) {
	for _, o := range m {
		o.ProviderSkipped(provider)
	}
}

func (m ProvidersMirrorMulti) MirroringProvider(provider string) {
	for _, o := range m {
		o.MirroringProvider(provider)
	}
}

func (m ProvidersMirrorMulti) ProviderVersionSelectedToMatchLockfile(provider string, version string) {
	for _, o := range m {
		o.ProviderVersionSelectedToMatchLockfile(provider, version)
	}
}

func (m ProvidersMirrorMulti) ProviderVersionSelectedToMatchConstraints(provider string, version string, constraints string) {
	for _, o := range m {
		o.ProviderVersionSelectedToMatchConstraints(provider, version, constraints)
	}
}

func (m ProvidersMirrorMulti) ProviderVersionSelectedWithNoConstraints(provider string, version string) {
	for _, o := range m {
		o.ProviderVersionSelectedWithNoConstraints(provider, version)
	}
}

func (m ProvidersMirrorMulti) DownloadingPackageFor(provider string, version string, platform string) {
	for _, o := range m {
		o.DownloadingPackageFor(provider, version, platform)
	}
}

func (m ProvidersMirrorMulti) PackageAuthenticated(provider string, version string, platform string, authResult string) {
	for _, o := range m {
		o.PackageAuthenticated(provider, version, platform, authResult)
	}
}

type ProvidersMirrorJSON struct {
	view *JSONView
}

var _ ProvidersMirror = (*ProvidersMirrorJSON)(nil)

func (v *ProvidersMirrorJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ProvidersMirrorJSON) ProviderSkipped(provider string) {
	v.view.Info(fmt.Sprintf("Skipping %s because it is built in to OpenTofu CLI", provider))
}

func (v *ProvidersMirrorJSON) MirroringProvider(provider string) {
	v.view.Info(fmt.Sprintf("Mirroring %s...", provider))
}

func (v *ProvidersMirrorJSON) ProviderVersionSelectedToMatchLockfile(provider string, version string) {
	v.view.Info(fmt.Sprintf("Selected %s v%s to match dependency lock file", provider, version))
}

func (v *ProvidersMirrorJSON) ProviderVersionSelectedToMatchConstraints(provider string, version string, constraints string) {
	v.view.Info(fmt.Sprintf("Selected %s v%s to meet constraints %s", provider, version, constraints))
}

func (v *ProvidersMirrorJSON) ProviderVersionSelectedWithNoConstraints(provider string, version string) {
	v.view.Info(fmt.Sprintf("Selected %s v%s with no constraints", provider, version))
}

func (v *ProvidersMirrorJSON) DownloadingPackageFor(provider string, version string, platform string) {
	v.view.Info(fmt.Sprintf("Downloading %s v%s package for %s...", provider, version, platform))
}

func (v *ProvidersMirrorJSON) PackageAuthenticated(provider string, version string, platform string, authResult string) {
	v.view.Info(fmt.Sprintf("Package %s v%s for %s authenticated: %s", provider, version, platform, authResult))
}
