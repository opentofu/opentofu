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

type ProvidersLock interface {
	Diagnostics(diags tfdiags.Diagnostics)
	InstallationFetching(provider string, version string, platform string)
	FetchPackageSuccess(keyID string, provider string, version string, platform string, auth string)
	LockUpdateNewProvider(provider string, platform string)
	LockUpdateNewHashForProvider(provider string, platform string)
	LockUpdateNoChange(provider string, platform string)
	UpdatedSuccessfully(madeAnyChange bool)
}

// NewProvidersLock returns an initialized ProvidersLock implementation for the given ViewType.
func NewProvidersLock(args arguments.ViewOptions, view *View) ProvidersLock {
	var ret ProvidersLock
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &ProvidersLockJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &ProvidersLockHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &ProvidersLockMulti{ret, &ProvidersLockJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

type ProvidersLockHuman struct {
	view *View
}

var _ ProvidersLock = (*ProvidersLockHuman)(nil)

func (v *ProvidersLockHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ProvidersLockHuman) InstallationFetching(provider string, version string, platform string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- Fetching %s %s for %s...", provider, version, platform))
}

func (v *ProvidersLockHuman) FetchPackageSuccess(keyID string, provider string, version string, platform string, auth string) {
	if keyID != "" {
		keyID = v.view.colorize.Color(fmt.Sprintf(", key ID [reset][bold]%s[reset]", keyID))
	}
	_, _ = v.view.streams.Println(fmt.Sprintf("- Retrieved %s %s for %s (%s%s)", provider, version, platform, auth, keyID))
}

func (v *ProvidersLockHuman) LockUpdateNewProvider(provider string, platform string) {
	_, _ = v.view.streams.Println(
		fmt.Sprintf(
			"- Obtained %s checksums for %s; This was a new provider and the checksums for this platform are now tracked in the lock file",
			provider,
			platform,
		),
	)
}

func (v *ProvidersLockHuman) LockUpdateNewHashForProvider(provider string, platform string) {
	_, _ = v.view.streams.Println(
		fmt.Sprintf(
			"- Obtained %s checksums for %s; Additional checksums for this platform are now tracked in the lock file",
			provider,
			platform,
		),
	)
}

func (v *ProvidersLockHuman) LockUpdateNoChange(provider string, platform string) {
	_, _ = v.view.streams.Println(
		fmt.Sprintf(
			"- Obtained %s checksums for %s; All checksums for this platform were already tracked in the lock file",
			provider,
			platform,
		),
	)
}

func (v *ProvidersLockHuman) UpdatedSuccessfully(madeAnyChange bool) {
	if !madeAnyChange {
		_, _ = v.view.streams.Println(v.view.colorize.Color("\n[bold][green]Success![reset] [bold]OpenTofu has validated the lock file and found no need for changes.[reset]"))
		return
	}
	_, _ = v.view.streams.Println(v.view.colorize.Color("\n[bold][green]Success![reset] [bold]OpenTofu has updated the lock file.[reset]"))
	_, _ = v.view.streams.Println("\nReview the changes in .terraform.lock.hcl and then commit to your\nversion control system to retain the new checksums.")
	// This new line was in the previous message but the built-in go linter does not allow it
	_, _ = v.view.streams.Println("")
}

type ProvidersLockMulti []ProvidersLock

var _ ProvidersLock = (ProvidersLockMulti)(nil)

func (m ProvidersLockMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m ProvidersLockMulti) InstallationFetching(provider string, version string, platform string) {
	for _, o := range m {
		o.InstallationFetching(provider, version, platform)
	}
}

func (m ProvidersLockMulti) FetchPackageSuccess(keyID string, provider string, version string, platform string, auth string) {
	for _, o := range m {
		o.FetchPackageSuccess(keyID, provider, version, platform, auth)
	}
}

func (m ProvidersLockMulti) LockUpdateNewProvider(provider string, platform string) {
	for _, o := range m {
		o.LockUpdateNewProvider(provider, platform)
	}
}

func (m ProvidersLockMulti) LockUpdateNewHashForProvider(provider string, platform string) {
	for _, o := range m {
		o.LockUpdateNewHashForProvider(provider, platform)
	}
}

func (m ProvidersLockMulti) LockUpdateNoChange(provider string, platform string) {
	for _, o := range m {
		o.LockUpdateNoChange(provider, platform)
	}
}

func (m ProvidersLockMulti) UpdatedSuccessfully(madeAnyChange bool) {
	for _, o := range m {
		o.UpdatedSuccessfully(madeAnyChange)
	}
}

type ProvidersLockJSON struct {
	view *JSONView
}

var _ ProvidersLock = (*ProvidersLockJSON)(nil)

func (v *ProvidersLockJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *ProvidersLockJSON) InstallationFetching(provider string, version string, platform string) {
	v.view.Info(fmt.Sprintf("Fetching %s %s for %s...", provider, version, platform))
}

func (v *ProvidersLockJSON) FetchPackageSuccess(keyID string, provider string, version string, platform string, auth string) {
	if keyID != "" {
		v.view.Info(fmt.Sprintf("Retrieved %s %s for %s (%s, key ID %s)", provider, version, platform, auth, keyID))
	} else {
		v.view.Info(fmt.Sprintf("Retrieved %s %s for %s (%s)", provider, version, platform, auth))
	}
}

func (v *ProvidersLockJSON) LockUpdateNewProvider(provider string, platform string) {
	v.view.Info(fmt.Sprintf("Obtained %s checksums for %s; This was a new provider and the checksums for this platform are now tracked in the lock file", provider, platform))
}

func (v *ProvidersLockJSON) LockUpdateNewHashForProvider(provider string, platform string) {
	v.view.Info(fmt.Sprintf("Obtained %s checksums for %s; Additional checksums for this platform are now tracked in the lock file", provider, platform))
}

func (v *ProvidersLockJSON) LockUpdateNoChange(provider string, platform string) {
	v.view.Info(fmt.Sprintf("Obtained %s checksums for %s; All checksums for this platform were already tracked in the lock file", provider, platform))
}

func (v *ProvidersLockJSON) UpdatedSuccessfully(madeAnyChange bool) {
	if !madeAnyChange {
		v.view.Info("Success! OpenTofu has validated the lock file and found no need for changes.")
		return
	}
	v.view.Info("Success! OpenTofu has updated the lock file. Review the changes in .terraform.lock.hcl and then commit to your version control system to retain the new checksums")
}
