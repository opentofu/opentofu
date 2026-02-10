// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/initwd"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Init interface {
	CopyFromModule(src string)
	InitialisedFromEmptyDir()

	Diagnostics(diags tfdiags.Diagnostics)
	HelpPrompt()

	ConfigError()
	OutputNewline()
	InitSuccess(cloud bool)
	InitSuccessCLI(cloud bool)

	InitializingModules(upgrade bool)

	InitializingCloudBackend()
	InitializingBackend()
	BackendTypeAlias(backendType, canonType string)

	InitializingProviderPlugins()
	ProviderAlreadyInstalled(provider string, version string, inCache bool)
	BuiltInProviderAvailable(provider string)
	ReusingLockFileVersion(provider string)
	FindingProviderVersions(provider string, constraints string)
	FindingLatestProviderVersion(provider string)
	UsingProviderFromCache(provider string, version string)
	InstallingProvider(provider string, version string, toCache bool)
	ProviderInstalled(provider string, version string, authResult string, keyID string)
	ProviderInstalledSkippedSignature(provider string, version string)
	WaitingForCacheLock(cacheDir string)
	ProvidersSignedInfo()
	ProviderUpgradeLockfileConflict()
	ProviderInstallationInterrupted()
	LockFileCreated()
	LockFileChanged()
	Hooks(showLocalDir bool) initwd.ModuleInstallHooks
}

// NewInit returns an initialized Init implementation for the given ViewType.
func NewInit(args arguments.ViewOptions, view *View) Init {
	var init Init
	switch args.ViewType {
	case arguments.ViewJSON:
		init = &InitJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		init = &InitHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		init = &InitMulti{init, &InitJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return init
}

type InitMulti []Init

var _ Init = (InitMulti)(nil)

func (m InitMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m InitMulti) HelpPrompt() {
	for _, o := range m {
		o.HelpPrompt()
	}
}

func (m InitMulti) CopyFromModule(src string) {
	for _, o := range m {
		o.CopyFromModule(src)
	}
}

func (m InitMulti) InitialisedFromEmptyDir() {
	for _, o := range m {
		o.InitialisedFromEmptyDir()
	}
}

func (m InitMulti) ConfigError() {
	for _, o := range m {
		o.ConfigError()
	}
}

func (m InitMulti) OutputNewline() {
	for _, o := range m {
		o.OutputNewline()
	}
}

func (m InitMulti) InitSuccess(cloud bool) {
	for _, o := range m {
		o.InitSuccess(cloud)
	}
}

func (m InitMulti) InitSuccessCLI(cloud bool) {
	for _, o := range m {
		o.InitSuccessCLI(cloud)
	}
}

func (m InitMulti) InitializingModules(upgrade bool) {
	for _, o := range m {
		o.InitializingModules(upgrade)
	}
}

func (m InitMulti) InitializingCloudBackend() {
	for _, o := range m {
		o.InitializingCloudBackend()
	}
}

func (m InitMulti) InitializingBackend() {
	for _, o := range m {
		o.InitializingBackend()
	}
}

func (m InitMulti) BackendTypeAlias(backendType, canonType string) {
	for _, o := range m {
		o.BackendTypeAlias(backendType, canonType)
	}
}

func (m InitMulti) InitializingProviderPlugins() {
	for _, o := range m {
		o.InitializingProviderPlugins()
	}
}

func (m InitMulti) ProviderAlreadyInstalled(provider string, version string, inCache bool) {
	for _, o := range m {
		o.ProviderAlreadyInstalled(provider, version, inCache)
	}
}

func (m InitMulti) BuiltInProviderAvailable(provider string) {
	for _, o := range m {
		o.BuiltInProviderAvailable(provider)
	}
}

func (m InitMulti) ReusingLockFileVersion(provider string) {
	for _, o := range m {
		o.ReusingLockFileVersion(provider)
	}
}

func (m InitMulti) FindingProviderVersions(provider string, constraints string) {
	for _, o := range m {
		o.FindingProviderVersions(provider, constraints)
	}
}

func (m InitMulti) FindingLatestProviderVersion(provider string) {
	for _, o := range m {
		o.FindingLatestProviderVersion(provider)
	}
}

func (m InitMulti) UsingProviderFromCache(provider string, version string) {
	for _, o := range m {
		o.UsingProviderFromCache(provider, version)
	}
}

func (m InitMulti) InstallingProvider(provider string, version string, toCache bool) {
	for _, o := range m {
		o.InstallingProvider(provider, version, toCache)
	}
}

func (m InitMulti) ProviderInstalled(provider string, version string, authResult string, keyID string) {
	for _, o := range m {
		o.ProviderInstalled(provider, version, authResult, keyID)
	}
}

func (m InitMulti) ProviderInstalledSkippedSignature(provider string, version string) {
	for _, o := range m {
		o.ProviderInstalledSkippedSignature(provider, version)
	}
}

func (m InitMulti) WaitingForCacheLock(cacheDir string) {
	for _, o := range m {
		o.WaitingForCacheLock(cacheDir)
	}
}

func (m InitMulti) ProvidersSignedInfo() {
	for _, o := range m {
		o.ProvidersSignedInfo()
	}
}

func (m InitMulti) ProviderUpgradeLockfileConflict() {
	for _, o := range m {
		o.ProviderUpgradeLockfileConflict()
	}
}

func (m InitMulti) ProviderInstallationInterrupted() {
	for _, o := range m {
		o.ProviderInstallationInterrupted()
	}
}

func (m InitMulti) LockFileCreated() {
	for _, o := range m {
		o.LockFileCreated()
	}
}

func (m InitMulti) LockFileChanged() {
	for _, o := range m {
		o.LockFileChanged()
	}
}

func (m InitMulti) Hooks(showLocalPath bool) initwd.ModuleInstallHooks {
	hooks := make([]initwd.ModuleInstallHooks, len(m))
	for i, o := range m {
		hooks[i] = o.Hooks(showLocalPath)
	}
	return moduleInstallationHookMulti(hooks)
}

type InitHuman struct {
	view *View
}

var _ Init = (*InitHuman)(nil)

func (v *InitHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *InitHuman) HelpPrompt() {
	v.view.HelpPrompt("init")
}

func (v *InitHuman) CopyFromModule(src string) {
	msg := v.view.colorize.Color(fmt.Sprintf("[reset][bold]Copying configuration[reset] from %q...", src))
	_, _ = v.view.streams.Println(msg)
}

func (v *InitHuman) InitialisedFromEmptyDir() {
	const outputInitEmpty = `
[reset][bold]OpenTofu initialized in an empty directory![reset]

The directory has no OpenTofu configuration files. You may begin working
with OpenTofu immediately by creating OpenTofu configuration files.`
	_, _ = v.view.streams.Println(strings.TrimSpace(v.view.colorize.Color(outputInitEmpty)))
}

func (v *InitHuman) ConfigError() {
	const errInitConfigError = `
[reset]OpenTofu encountered problems during initialization, including problems
with the configuration, described below.

The OpenTofu configuration must be valid before initialization so that
OpenTofu can determine which modules and providers need to be installed.`

	_, _ = v.view.streams.Eprintln(v.view.colorize.Color(errInitConfigError))
}

func (v *InitHuman) OutputNewline() {
	_, _ = v.view.streams.Println("")
}

func (v *InitHuman) InitSuccess(cloud bool) {
	if cloud {
		const outputInitSuccessCloud = `[reset][bold][green]Cloud backend has been successfully initialized![reset][green]`
		_, _ = v.view.streams.Println(v.view.colorize.Color(outputInitSuccessCloud))
	} else {
		const outputInitSuccess = `[reset][bold][green]OpenTofu has been successfully initialized![reset][green]`
		_, _ = v.view.streams.Println(v.view.colorize.Color(outputInitSuccess))
	}
}

func (v *InitHuman) InitSuccessCLI(cloud bool) {
	if cloud {
		const outputInitSuccessCLICloud = `[reset][green]
You may now begin working with cloud backend. Try running "tofu plan" to
see any changes that are required for your infrastructure.

If you ever set or change modules or OpenTofu Settings, run "tofu init"
again to reinitialize your working directory.`
		_, _ = v.view.streams.Println(v.view.colorize.Color(outputInitSuccessCLICloud))
	} else {
		const outputInitSuccessCLI = `[reset][green]
You may now begin working with OpenTofu. Try running "tofu plan" to see
any changes that are required for your infrastructure. All OpenTofu commands
should now work.

If you ever set or change modules or backend configuration for OpenTofu,
rerun this command to reinitialize your working directory. If you forget, other
commands will detect it and remind you to do so if necessary.`
		_, _ = v.view.streams.Println(v.view.colorize.Color(outputInitSuccessCLI))
	}
}

func (v *InitHuman) InitializingModules(upgrade bool) {
	if upgrade {
		_, _ = v.view.streams.Println(v.view.colorize.Color("[reset][bold]Upgrading modules..."))
	} else {
		_, _ = v.view.streams.Println(v.view.colorize.Color("[reset][bold]Initializing modules..."))
	}
}

func (v *InitHuman) InitializingCloudBackend() {
	_, _ = v.view.streams.Println(v.view.colorize.Color("\n[reset][bold]Initializing cloud backend..."))
}

func (v *InitHuman) InitializingBackend() {
	_, _ = v.view.streams.Println(v.view.colorize.Color("\n[reset][bold]Initializing the backend..."))
}

func (v *InitHuman) BackendTypeAlias(backendType, canonType string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- %q is an alias for backend type %q", backendType, canonType))
}

func (v *InitHuman) InitializingProviderPlugins() {
	_, _ = v.view.streams.Println(v.view.colorize.Color("\n[reset][bold]Initializing provider plugins..."))
}

func (v *InitHuman) ProviderAlreadyInstalled(provider string, version string, inCache bool) {
	if inCache {
		_, _ = v.view.streams.Println(fmt.Sprintf("- Detected previously-installed %s v%s in the shared cache directory", provider, version))
	} else {
		_, _ = v.view.streams.Println(fmt.Sprintf("- Using previously-installed %s v%s", provider, version))
	}
}

func (v *InitHuman) BuiltInProviderAvailable(provider string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- %s is built in to OpenTofu", provider))
}

func (v *InitHuman) ReusingLockFileVersion(provider string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- Reusing previous version of %s from the dependency lock file", provider))
}

func (v *InitHuman) FindingProviderVersions(provider string, constraints string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- Finding %s versions matching %q...", provider, constraints))
}

func (v *InitHuman) FindingLatestProviderVersion(provider string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- Finding latest version of %s...", provider))
}

func (v *InitHuman) UsingProviderFromCache(provider string, version string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- Using %s v%s from the shared cache directory", provider, version))
}

func (v *InitHuman) InstallingProvider(provider string, version string, toCache bool) {
	if toCache {
		_, _ = v.view.streams.Println(fmt.Sprintf("- Installing %s v%s to the shared cache directory...", provider, version))
	} else {
		_, _ = v.view.streams.Println(fmt.Sprintf("- Installing %s v%s...", provider, version))
	}
}

func (v *InitHuman) ProviderInstalled(provider string, version string, authResult string, keyID string) {
	if keyID != "" {
		keyID = v.view.colorize.Color(fmt.Sprintf(", key ID [reset][bold]%s[reset]", keyID))
	}
	_, _ = v.view.streams.Println(fmt.Sprintf("- Installed %s v%s (%s%s)", provider, version, authResult, keyID))
}

func (v *InitHuman) ProviderInstalledSkippedSignature(provider string, version string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- Installed %s v%s. Signature validation was skipped due to the registry not containing GPG keys for this provider", provider, version))
}

func (v *InitHuman) WaitingForCacheLock(cacheDir string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- Waiting for lock on cache directory %s", cacheDir))
}

func (v *InitHuman) ProvidersSignedInfo() {
	_, _ = v.view.streams.Println(fmt.Sprintf("\nProviders are signed by their developers.\n" +
		"If you'd like to know more about provider signing, you can read about it here:\n" +
		"https://opentofu.org/docs/cli/plugins/signing/"))
}

func (v *InitHuman) ProviderUpgradeLockfileConflict() {
	_, _ = v.view.streams.Eprintln("The -upgrade flag conflicts with -lockfile=readonly.")
}

func (v *InitHuman) ProviderInstallationInterrupted() {
	_, _ = v.view.streams.Eprintln("Provider installation was canceled by an interrupt signal.")
}

func (v *InitHuman) LockFileCreated() {
	_, _ = v.view.streams.Println(v.view.colorize.Color(`
OpenTofu has created a lock file [bold].terraform.lock.hcl[reset] to record the provider
selections it made above. Include this file in your version control repository
so that OpenTofu can guarantee to make the same selections by default when
you run "tofu init" in the future.`))
}

func (v *InitHuman) LockFileChanged() {
	_, _ = v.view.streams.Println(v.view.colorize.Color(`
OpenTofu has made some changes to the provider dependency selections recorded
in the .terraform.lock.hcl file. Review those changes and commit them to your
version control system if they represent changes you intended to make.`))
}

func (v *InitHuman) Hooks(showLocalPath bool) initwd.ModuleInstallHooks {
	return &moduleInstallationHookHuman{
		v:              v.view,
		showLocalPaths: showLocalPath,
	}
}

type InitJSON struct {
	view *JSONView
}

var _ Init = (*InitJSON)(nil)

func (v *InitJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *InitJSON) HelpPrompt() {}

func (v *InitJSON) CopyFromModule(src string) {
	v.view.Info(fmt.Sprintf("Copying configuration from %q...", src))
}

func (v *InitJSON) InitialisedFromEmptyDir() {
	const outputInitEmpty = `OpenTofu initialized in an empty directory! The directory has no OpenTofu configuration files. You may begin working with OpenTofu immediately by creating OpenTofu configuration files.`
	v.view.Info(outputInitEmpty)
}

func (v *InitJSON) ConfigError() {
	const errInitConfigError = `OpenTofu encountered problems during initialization, including problems with the configuration, described below. The OpenTofu configuration must be valid before initialization so that OpenTofu can determine which modules and providers need to be installed.`
	v.view.Error(errInitConfigError)
}

func (v *InitJSON) OutputNewline() {
}

func (v *InitJSON) InitSuccess(cloud bool) {
	if cloud {
		v.view.Info(`Cloud backend has been successfully initialized!`)
	} else {
		v.view.Info(`OpenTofu has been successfully initialized!`)
	}
}

func (v *InitJSON) InitSuccessCLI(cloud bool) {
	if cloud {
		const outputInitSuccessCLICloud = `You may now begin working with cloud backend. Try running "tofu plan" to see any changes that are required for your infrastructure. If you ever set or change modules or OpenTofu Settings, run "tofu init" again to reinitialize your working directory.`
		v.view.Info(outputInitSuccessCLICloud)
	} else {
		const outputInitSuccessCLI = `You may now begin working with OpenTofu. Try running "tofu plan" to see any changes that are required for your infrastructure. All OpenTofu commands should now work. If you ever set or change modules or backend configuration for OpenTofu, rerun this command to reinitialize your working directory. If you forget, other commands will detect it and remind you to do so if necessary.`
		v.view.Info(outputInitSuccessCLI)
	}
}

func (v *InitJSON) InitializingModules(upgrade bool) {
	if upgrade {
		v.view.Info("Upgrading modules...")
	} else {
		v.view.Info("Initializing modules...")
	}
}

func (v *InitJSON) InitializingCloudBackend() {
	v.view.Info("Initializing cloud backend...")
}

func (v *InitJSON) InitializingBackend() {
	v.view.Info("Initializing the backend...")
}

func (v *InitJSON) BackendTypeAlias(backendType, canonType string) {
	v.view.Info(fmt.Sprintf("%q is an alias for backend type %q", backendType, canonType))
}

func (v *InitJSON) InitializingProviderPlugins() {
	v.view.Info("Initializing provider plugins...")
}

func (v *InitJSON) ProviderAlreadyInstalled(provider string, version string, inCache bool) {
	if inCache {
		v.view.Info(fmt.Sprintf("Detected previously-installed %s v%s in the shared cache directory", provider, version))
	} else {
		v.view.Info(fmt.Sprintf("Using previously-installed %s v%s", provider, version))
	}
}

func (v *InitJSON) BuiltInProviderAvailable(provider string) {
	v.view.Info(fmt.Sprintf("%s is built in to OpenTofu", provider))
}

func (v *InitJSON) ReusingLockFileVersion(provider string) {
	v.view.Info(fmt.Sprintf("Reusing previous version of %s from the dependency lock file", provider))
}

func (v *InitJSON) FindingProviderVersions(provider string, constraints string) {
	v.view.Info(fmt.Sprintf("Finding %s versions matching %q...", provider, constraints))
}

func (v *InitJSON) FindingLatestProviderVersion(provider string) {
	v.view.Info(fmt.Sprintf("Finding latest version of %s...", provider))
}

func (v *InitJSON) UsingProviderFromCache(provider string, version string) {
	v.view.Info(fmt.Sprintf("Using %s v%s from the shared cache directory", provider, version))
}

func (v *InitJSON) InstallingProvider(provider string, version string, toCache bool) {
	if toCache {
		v.view.Info(fmt.Sprintf("Installing %s v%s to the shared cache directory...", provider, version))
	} else {
		v.view.Info(fmt.Sprintf("Installing %s v%s...", provider, version))
	}
}

func (v *InitJSON) ProviderInstalled(provider string, version string, authResult string, keyID string) {
	if keyID != "" {
		keyID = fmt.Sprintf(", key ID %s", keyID)
	}
	v.view.Info(fmt.Sprintf("Installed %s v%s (%s%s)", provider, version, authResult, keyID))
}

func (v *InitJSON) ProviderInstalledSkippedSignature(provider string, version string) {
	v.view.Warn(fmt.Sprintf("Installed %s v%s. Signature validation was skipped due to the registry not containing GPG keys for this provider", provider, version))
}

func (v *InitJSON) WaitingForCacheLock(cacheDir string) {
	v.view.Info(fmt.Sprintf("Waiting for lock on cache directory %s", cacheDir))
}

func (v *InitJSON) ProvidersSignedInfo() {
	v.view.Info("Providers are signed by their developers. " +
		"If you'd like to know more about provider signing, you can read about it here: " +
		"https://opentofu.org/docs/cli/plugins/signing/")
}

func (v *InitJSON) ProviderUpgradeLockfileConflict() {
	v.view.Error("The -upgrade flag conflicts with -lockfile=readonly.")
}

func (v *InitJSON) ProviderInstallationInterrupted() {
	v.view.Error("Provider installation was canceled by an interrupt signal.")
}

func (v *InitJSON) LockFileCreated() {
	v.view.Info("OpenTofu has created a lock file .terraform.lock.hcl to record the provider " +
		"selections it made above. Include this file in your version control repository " +
		"so that OpenTofu can guarantee to make the same selections by default when " +
		"you run \"tofu init\" in the future.")
}

func (v *InitJSON) LockFileChanged() {
	v.view.Info("OpenTofu has made some changes to the provider dependency selections recorded " +
		"in the .terraform.lock.hcl file. Review those changes and commit them to your " +
		"version control system if they represent changes you intended to make.")
}

func (v *InitJSON) Hooks(showLocalPath bool) initwd.ModuleInstallHooks {
	return &moduleInstallationHookJSON{
		v:              v.view,
		showLocalPaths: showLocalPath,
	}
}
