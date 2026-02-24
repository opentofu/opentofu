// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestInitViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(init Init)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		"copyFromModule": {
			viewCall: func(init Init) {
				init.CopyFromModule("my source")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Copying configuration from \"my source\"...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`Copying configuration from "my source"...`),
		},
		"fromEmptyDir": {
			viewCall: func(init Init) {
				init.InitialisedFromEmptyDir()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "OpenTofu initialized in an empty directory! The directory has no OpenTofu configuration files. You may begin working with OpenTofu immediately by creating OpenTofu configuration files.",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("OpenTofu initialized in an empty directory!\n\nThe directory has no OpenTofu configuration files. You may begin working\nwith OpenTofu immediately by creating OpenTofu configuration files."),
		},
		"outputNewline": {
			viewCall: func(init Init) {
				init.OutputNewline()
			},
			wantStdout: withNewline(""),
			wantStderr: "",
			wantJson:   []map[string]any{{}},
		},
		"initSuccess_cloud": {
			viewCall: func(init Init) {
				init.InitSuccess(true)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Cloud backend has been successfully initialized!",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Cloud backend has been successfully initialized!"),
		},
		"initSuccess_non-cloud": {
			viewCall: func(init Init) {
				init.InitSuccess(false)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "OpenTofu has been successfully initialized!",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("OpenTofu has been successfully initialized!"),
		},
		"initSuccessCLI_cloud": {
			viewCall: func(init Init) {
				init.InitSuccessCLI(true)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "You may now begin working with cloud backend. Try running \"tofu plan\" to see any changes that are required for your infrastructure. If you ever set or change modules or OpenTofu Settings, run \"tofu init\" again to reinitialize your working directory.",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("\nYou may now begin working with cloud backend. Try running \"tofu plan\" to\nsee any changes that are required for your infrastructure.\n\nIf you ever set or change modules or OpenTofu Settings, run \"tofu init\"\nagain to reinitialize your working directory."),
		},
		"initSuccessCLI_non-cloud": {
			viewCall: func(init Init) {
				init.InitSuccessCLI(false)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "You may now begin working with OpenTofu. Try running \"tofu plan\" to see any changes that are required for your infrastructure. All OpenTofu commands should now work. If you ever set or change modules or backend configuration for OpenTofu, rerun this command to reinitialize your working directory. If you forget, other commands will detect it and remind you to do so if necessary.",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("\nYou may now begin working with OpenTofu. Try running \"tofu plan\" to see\nany changes that are required for your infrastructure. All OpenTofu commands\nshould now work.\n\nIf you ever set or change modules or backend configuration for OpenTofu,\nrerun this command to reinitialize your working directory. If you forget, other\ncommands will detect it and remind you to do so if necessary."),
		},
		"initializingModules_upgrade": {
			viewCall: func(init Init) {
				init.InitializingModules(true)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Upgrading modules...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Upgrading modules..."),
		},
		"initializingModules_init": {
			viewCall: func(init Init) {
				init.InitializingModules(false)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Initializing modules...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Initializing modules..."),
		},
		"initializingCloudBackend": {
			viewCall: func(init Init) {
				init.InitializingCloudBackend()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Initializing cloud backend...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("\nInitializing cloud backend..."),
		},
		"initializingBackend": {
			viewCall: func(init Init) {
				init.InitializingBackend()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Initializing the backend...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("\nInitializing the backend..."),
		},
		"backendTypeAlias": {
			viewCall: func(init Init) {
				init.BackendTypeAlias("s3", "aws_s3")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "\"s3\" is an alias for backend type \"aws_s3\"",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- \"s3\" is an alias for backend type \"aws_s3\""),
		},
		"initializingProviderPlugins": {
			viewCall: func(init Init) {
				init.InitializingProviderPlugins()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Initializing provider plugins...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("\nInitializing provider plugins..."),
		},
		"providerAlreadyInstalled_notInCache": {
			viewCall: func(init Init) {
				init.ProviderAlreadyInstalled("hashicorp/aws", "5.0.0", false)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Using previously-installed hashicorp/aws v5.0.0",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Using previously-installed hashicorp/aws v5.0.0"),
		},
		"providerAlreadyInstalled_inCache": {
			viewCall: func(init Init) {
				init.ProviderAlreadyInstalled("hashicorp/aws", "5.0.0", true)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Detected previously-installed hashicorp/aws v5.0.0 in the shared cache directory",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Detected previously-installed hashicorp/aws v5.0.0 in the shared cache directory"),
		},
		"builtInProviderAvailable": {
			viewCall: func(init Init) {
				init.BuiltInProviderAvailable("terraform")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "terraform is built in to OpenTofu",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- terraform is built in to OpenTofu"),
		},
		"reusingLockFileVersion": {
			viewCall: func(init Init) {
				init.ReusingLockFileVersion("hashicorp/random")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Reusing previous version of hashicorp/random from the dependency lock file",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Reusing previous version of hashicorp/random from the dependency lock file"),
		},
		"findingProviderVersions": {
			viewCall: func(init Init) {
				init.FindingProviderVersions("hashicorp/aws", "~> 5.0")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Finding hashicorp/aws versions matching \"~> 5.0\"...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Finding hashicorp/aws versions matching \"~> 5.0\"..."),
		},
		"findingLatestProviderVersion": {
			viewCall: func(init Init) {
				init.FindingLatestProviderVersion("hashicorp/null")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Finding latest version of hashicorp/null...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Finding latest version of hashicorp/null..."),
		},
		"usingProviderFromCache": {
			viewCall: func(init Init) {
				init.UsingProviderFromCache("hashicorp/aws", "5.0.0")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Using hashicorp/aws v5.0.0 from the shared cache directory",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Using hashicorp/aws v5.0.0 from the shared cache directory"),
		},
		"installingProvider_notToCache": {
			viewCall: func(init Init) {
				init.InstallingProvider("hashicorp/aws", "5.0.0", false)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Installing hashicorp/aws v5.0.0...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Installing hashicorp/aws v5.0.0..."),
		},
		"installingProvider_toCache": {
			viewCall: func(init Init) {
				init.InstallingProvider("hashicorp/aws", "5.0.0", true)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Installing hashicorp/aws v5.0.0 to the shared cache directory...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Installing hashicorp/aws v5.0.0 to the shared cache directory..."),
		},
		"providerInstalled_noKeyID": {
			viewCall: func(init Init) {
				init.ProviderInstalled("hashicorp/aws", "5.0.0", "signed by HashiCorp", "")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Installed hashicorp/aws v5.0.0 (signed by HashiCorp)",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Installed hashicorp/aws v5.0.0 (signed by HashiCorp)"),
		},
		"providerInstalled_withKeyID": {
			viewCall: func(init Init) {
				init.ProviderInstalled("hashicorp/aws", "5.0.0", "signed", "34365D9472D7468F")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Installed hashicorp/aws v5.0.0 (signed, key ID 34365D9472D7468F)",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Installed hashicorp/aws v5.0.0 (signed, key ID 34365D9472D7468F)"),
		},
		"waitingForCacheLock": {
			viewCall: func(init Init) {
				init.WaitingForCacheLock("/tmp/plugin-cache")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Waiting for lock on cache directory /tmp/plugin-cache",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Waiting for lock on cache directory /tmp/plugin-cache"),
		},
		"providersSignedInfo": {
			viewCall: func(init Init) {
				init.ProvidersSignedInfo()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Providers are signed by their developers. If you'd like to know more about provider signing, you can read about it here: https://opentofu.org/docs/cli/plugins/signing/",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("\nProviders are signed by their developers.\nIf you'd like to know more about provider signing, you can read about it here:\nhttps://opentofu.org/docs/cli/plugins/signing/"),
		},
		"lockFileCreated": {
			viewCall: func(init Init) {
				init.LockFileCreated()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "OpenTofu has created a lock file .terraform.lock.hcl to record the provider selections it made above. Include this file in your version control repository so that OpenTofu can guarantee to make the same selections by default when you run \"tofu init\" in the future.",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("\nOpenTofu has created a lock file .terraform.lock.hcl to record the provider\nselections it made above. Include this file in your version control repository\nso that OpenTofu can guarantee to make the same selections by default when\nyou run \"tofu init\" in the future."),
		},
		"lockFileChanged": {
			viewCall: func(init Init) {
				init.LockFileChanged()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "OpenTofu has made some changes to the provider dependency selections recorded in the .terraform.lock.hcl file. Review those changes and commit them to your version control system if they represent changes you intended to make.",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("\nOpenTofu has made some changes to the provider dependency selections recorded\nin the .terraform.lock.hcl file. Review those changes and commit them to your\nversion control system if they represent changes you intended to make."),
		},
		// to stderr
		"configError": {
			viewCall: func(init Init) {
				init.ConfigError()
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "OpenTofu encountered problems during initialization, including problems with the configuration, described below. The OpenTofu configuration must be valid before initialization so that OpenTofu can determine which modules and providers need to be installed.",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: "",
			wantStderr: withNewline("\nOpenTofu encountered problems during initialization, including problems\nwith the configuration, described below.\n\nThe OpenTofu configuration must be valid before initialization so that\nOpenTofu can determine which modules and providers need to be installed."),
		},
		"providerInstalledSkippedSignature": {
			viewCall: func(init Init) {
				init.ProviderInstalledSkippedSignature("hashicorp/random", "3.0.0")
			},
			wantJson: []map[string]any{
				{
					"@level":   "warn",
					"@message": "Installed hashicorp/random v3.0.0. Signature validation was skipped due to the registry not containing GPG keys for this provider",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- Installed hashicorp/random v3.0.0. Signature validation was skipped due to the registry not containing GPG keys for this provider"),
			wantStderr: "",
		},
		"providerUpgradeLockfileConflict": {
			viewCall: func(init Init) {
				init.ProviderUpgradeLockfileConflict()
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "The -upgrade flag conflicts with -lockfile=readonly.",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: "",
			wantStderr: withNewline("The -upgrade flag conflicts with -lockfile=readonly."),
		},
		"providerInstallationInterrupted": {
			viewCall: func(init Init) {
				init.ProviderInstallationInterrupted()
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "Provider installation was canceled by an interrupt signal.",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: "",
			wantStderr: withNewline("Provider installation was canceled by an interrupt signal."),
		},
		// Diagnostics
		"warning": {
			viewCall: func(init Init) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "foo bar"),
				}
				init.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning occurred\n\nfoo bar"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "warn",
					"@message": "Warning: A warning occurred",
					"@module":  "tofu.ui",
					"diagnostic": map[string]any{
						"detail":   "foo bar",
						"severity": "warning",
						"summary":  "A warning occurred",
					},
					"type": "diagnostic",
				},
			},
		},
		"error": {
			viewCall: func(init Init) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "foo bar"),
				}
				init.Diagnostics(diags)
			},
			wantStdout: "",
			wantStderr: withNewline("\nError: An error occurred\n\nfoo bar"),
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "Error: An error occurred",
					"@module":  "tofu.ui",
					"diagnostic": map[string]any{
						"detail":   "foo bar",
						"severity": "error",
						"summary":  "An error occurred",
					},
					"type": "diagnostic",
				},
			},
		},
		"multiple_diagnostics": {
			viewCall: func(init Init) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				init.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning\n\nfoo bar warning"),
			wantStderr: withNewline("\nError: An error\n\nfoo bar error"),
			wantJson: []map[string]any{
				{
					"@level":   "warn",
					"@message": "Warning: A warning",
					"@module":  "tofu.ui",
					"diagnostic": map[string]any{
						"detail":   "foo bar warning",
						"severity": "warning",
						"summary":  "A warning",
					},
					"type": "diagnostic",
				},
				{
					"@level":   "error",
					"@message": "Error: An error",
					"@module":  "tofu.ui",
					"diagnostic": map[string]any{
						"detail":   "foo bar error",
						"severity": "error",
						"summary":  "An error",
					},
					"type": "diagnostic",
				},
			},
		},
		// Miscs
		"help prompt": {
			viewCall: func(init Init) {
				init.HelpPrompt()
			},
			wantStdout: "",
			wantStderr: withNewline("\nFor more help on using this command, run:\n  tofu init -help"),
			wantJson:   []map[string]any{{}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testInitHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testInitJson(t, tc.viewCall, tc.wantJson)
			testInitMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func TestInitViews_Hooks(t *testing.T) {
	t.Run("hooks_human_withLocalPath", func(t *testing.T) {
		view, _ := testView(t)
		initView := NewInit(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
		hooks := initView.Hooks(true)

		if hooks == nil {
			t.Fatal("expected hooks to be non-nil")
		}

		// Verify it's the right type
		_, ok := hooks.(*moduleInstallationHookHuman)
		if !ok {
			t.Errorf("expected *moduleInstallationHookHuman, got %T", hooks)
		}
	})

	t.Run("hooks_human_withoutLocalPath", func(t *testing.T) {
		view, _ := testView(t)
		initView := NewInit(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
		hooks := initView.Hooks(false)

		if hooks == nil {
			t.Fatal("expected hooks to be non-nil")
		}

		_, ok := hooks.(*moduleInstallationHookHuman)
		if !ok {
			t.Errorf("expected *moduleInstallationHookHuman, got %T", hooks)
		}
	})

	t.Run("hooks_json", func(t *testing.T) {
		view, _ := testView(t)
		initView := NewInit(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
		hooks := initView.Hooks(true)

		if hooks == nil {
			t.Fatal("expected hooks to be non-nil")
		}

		_, ok := hooks.(*moduleInstallationHookJSON)
		if !ok {
			t.Errorf("expected *moduleInstallationHookJSON, got %T", hooks)
		}
	})

	t.Run("hooks_multi", func(t *testing.T) {
		jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
		if err != nil {
			t.Fatalf("failed to create temp file: %s", err)
		}
		defer jsonInto.Close()

		view, _ := testView(t)
		initView := NewInit(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
		hooks := initView.Hooks(true)

		if hooks == nil {
			t.Fatal("expected hooks to be non-nil")
		}

		// Should be multi hook
		_, ok := hooks.(moduleInstallationHookMulti)
		if !ok {
			t.Errorf("expected moduleInstallationHookMulti, got %T", hooks)
		}
	})
}

func testInitHuman(t *testing.T, call func(init Init), wantStdout, wantStderr string) {
	view, done := testView(t)
	initView := NewInit(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(initView)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testInitJson(t *testing.T, call func(init Init), want []map[string]interface{}) {
	// New type just to assert the fields that we are interested in
	view, done := testView(t)
	initView := NewInit(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(initView)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testInitMulti(t *testing.T, call func(init Init), wantStdout string, wantStderr string, want []map[string]interface{}) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	initView := NewInit(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
	call(initView)
	{
		if err := jsonInto.Close(); err != nil {
			t.Fatalf("failed to close the jsonInto file: %s", err)
		}
		// check the fileInto content
		fileContent, err := os.ReadFile(jsonInto.Name())
		if err != nil {
			t.Fatalf("failed to read the file content with the json output: %s", err)
		}
		testJSONViewOutputEquals(t, string(fileContent), want)
	}
	{
		// check the human output
		output := done(t)
		if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
			t.Errorf("invalid stderr (-want, +got):\n%s", diff)
		}
		if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
			t.Errorf("invalid stdout (-want, +got):\n%s", diff)
		}
	}
}

func testView(t *testing.T) (*View, func(*testing.T) *terminal.TestOutput) {
	streams, done := terminal.StreamsForTesting(t)
	view := NewView(streams)
	return view, done
}

func withNewline(in string) string {
	return fmt.Sprintf("%s\n", in)
}
