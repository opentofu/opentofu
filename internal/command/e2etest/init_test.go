// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/opentofu/opentofu/internal/e2e"
)

func TestInitProviders(t *testing.T) {
	t.Parallel()

	// This test reaches out to registry.opentofu.org to download the
	// template provider, so it can only run if network access is allowed.
	// We intentionally don't try to stub this here, because there's already
	// a stubbed version of this in the "command" package and so the goal here
	// is to test the interaction with the real repository.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "template-provider")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	stdout, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	if !strings.Contains(stdout, "OpenTofu has been successfully initialized!") {
		t.Errorf("success message is missing from output:\n%s", stdout)
	}

	if !strings.Contains(stdout, "- Installing hashicorp/template v") {
		t.Errorf("provider download message is missing from output:\n%s", stdout)
		t.Logf("(this can happen if you have a copy of the plugin in one of the global plugin search dirs)")
	}

	if !strings.Contains(stdout, "OpenTofu has created a lock file") {
		t.Errorf("lock file notification is missing from output:\n%s", stdout)
	}

}

func TestInitProvidersInternal(t *testing.T) {
	t.Parallel()

	// This test should _not_ reach out anywhere because the "terraform"
	// provider is internal to the core tofu binary.

	t.Run("output in human readable format", func(t *testing.T) {
		fixturePath := filepath.Join("testdata", "tf-provider")
		tf := e2e.NewBinary(t, tofuBin, fixturePath)

		stdout, stderr, err := tf.Run("init")
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}

		if stderr != "" {
			t.Errorf("unexpected stderr output:\n%s", stderr)
		}

		if !strings.Contains(stdout, "OpenTofu has been successfully initialized!") {
			t.Errorf("success message is missing from output:\n%s", stdout)
		}

		if strings.Contains(stdout, "Installing hashicorp/terraform") {
			// Shouldn't have downloaded anything with this config, because the
			// provider is built in.
			t.Errorf("provider download message appeared in output:\n%s", stdout)
		}

		if strings.Contains(stdout, "Installing terraform.io/builtin/terraform") {
			// Shouldn't have downloaded anything with this config, because the
			// provider is built in.
			t.Errorf("provider download message appeared in output:\n%s", stdout)
		}
	})

	t.Run("output in machine readable format", func(t *testing.T) {
		fixturePath := filepath.Join("testdata", "tf-provider")
		tf := e2e.NewBinary(t, tofuBin, fixturePath)

		stdout, stderr, err := tf.Run("init", "-json")
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}

		if stderr != "" {
			t.Errorf("unexpected stderr output:\n%s", stderr)
		}

		// we can not check timestamp, so the sub string is not a valid json object
		if !strings.Contains(stdout, `{"@level":"info","@message":"OpenTofu has been successfully initialized!","@module":"tofu.ui"`) {
			t.Errorf("success message is missing from output:\n%s", stdout)
		}

		if strings.Contains(stdout, "Installing hashicorp/terraform") {
			// Shouldn't have downloaded anything with this config, because the
			// provider is built in.
			t.Errorf("provider download message appeared in output:\n%s", stdout)
		}

		if strings.Contains(stdout, "Installing terraform.io/builtin/terraform") {
			// Shouldn't have downloaded anything with this config, because the
			// provider is built in.
			t.Errorf("provider download message appeared in output:\n%s", stdout)
		}
	})

}

func TestInitProvidersVendored(t *testing.T) {
	t.Parallel()

	// This test will try to reach out to registry.opentofu.org as one of the
	// possible installation locations for
	// hashicorp/null, where it will find that
	// versions do exist but will ultimately select the version that is
	// vendored due to the version constraint.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "vendored-provider")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// Our fixture dir has a generic os_arch dir, which we need to customize
	// to the actual OS/arch where this test is running in order to get the
	// desired result.
	fixtMachineDir := tf.Path("terraform.d/plugins/registry.opentofu.org/hashicorp/null/1.0.0+local/os_arch")
	wantMachineDir := tf.Path("terraform.d/plugins/registry.opentofu.org/hashicorp/null/1.0.0+local/", fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH))
	err := os.Rename(fixtMachineDir, wantMachineDir)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	stdout, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	if !strings.Contains(stdout, "OpenTofu has been successfully initialized!") {
		t.Errorf("success message is missing from output:\n%s", stdout)
	}

	if !strings.Contains(stdout, "- Installing hashicorp/null v1.0.0+local") {
		t.Errorf("provider download message is missing from output:\n%s", stdout)
		t.Logf("(this can happen if you have a copy of the plugin in one of the global plugin search dirs)")
	}

}

func TestInitProvidersLocalOnly(t *testing.T) {
	t.Parallel()

	// This test should not reach out to the network if it is behaving as
	// intended. If it _does_ try to access an upstream registry and encounter
	// an error doing so then that's a legitimate test failure that should be
	// fixed. (If it incorrectly reaches out anywhere then it's likely to be
	// to the host "example.com", which is the placeholder domain we use in
	// the test fixture.)

	t.Run("output in human readable format", func(t *testing.T) {
		fixturePath := filepath.Join("testdata", "local-only-provider")
		tf := e2e.NewBinary(t, tofuBin, fixturePath)
		// If you run this test on a workstation with a plugin-cache directory
		// configured, it will leave a bad directory behind and tofu init will
		// not work until you remove it.
		//
		// To avoid this, we will  "zero out" any existing cli config file.
		tf.AddEnv("TF_CLI_CONFIG_FILE=")

		// Our fixture dir has a generic os_arch dir, which we need to customize
		// to the actual OS/arch where this test is running in order to get the
		// desired result.
		fixtMachineDir := tf.Path("terraform.d/plugins/example.com/awesomecorp/happycloud/1.2.0/os_arch")
		wantMachineDir := tf.Path("terraform.d/plugins/example.com/awesomecorp/happycloud/1.2.0/", fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH))
		err := os.Rename(fixtMachineDir, wantMachineDir)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		stdout, stderr, err := tf.Run("init")
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}

		if stderr != "" {
			t.Errorf("unexpected stderr output:\n%s", stderr)
		}

		if !strings.Contains(stdout, "OpenTofu has been successfully initialized!") {
			t.Errorf("success message is missing from output:\n%s", stdout)
		}

		if !strings.Contains(stdout, "- Installing example.com/awesomecorp/happycloud v1.2.0") {
			t.Errorf("provider download message is missing from output:\n%s", stdout)
			t.Logf("(this can happen if you have a conflicting copy of the plugin in one of the global plugin search dirs)")
		}
	})

	t.Run("output in machine readable format", func(t *testing.T) {
		fixturePath := filepath.Join("testdata", "local-only-provider")
		tf := e2e.NewBinary(t, tofuBin, fixturePath)
		// If you run this test on a workstation with a plugin-cache directory
		// configured, it will leave a bad directory behind and tofu init will
		// not work until you remove it.
		//
		// To avoid this, we will  "zero out" any existing cli config file.
		tf.AddEnv("TF_CLI_CONFIG_FILE=")

		// Our fixture dir has a generic os_arch dir, which we need to customize
		// to the actual OS/arch where this test is running in order to get the
		// desired result.
		fixtMachineDir := tf.Path("terraform.d/plugins/example.com/awesomecorp/happycloud/1.2.0/os_arch")
		wantMachineDir := tf.Path("terraform.d/plugins/example.com/awesomecorp/happycloud/1.2.0/", fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH))
		err := os.Rename(fixtMachineDir, wantMachineDir)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		stdout, stderr, err := tf.Run("init", "-json")
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}

		if stderr != "" {
			t.Errorf("unexpected stderr output:\n%s", stderr)
		}

		// we can not check timestamp, so the sub string is not a valid json object
		if !strings.Contains(stdout, `{"@level":"info","@message":"OpenTofu has been successfully initialized!","@module":"tofu.ui"`) {
			t.Errorf("success message is missing from output:\n%s", stdout)
		}

		if !strings.Contains(stdout, `{"@level":"info","@message":"- Installing example.com/awesomecorp/happycloud v1.2.0...","@module":"tofu.ui"`) {
			t.Errorf("provider download message is missing from output:\n%s", stdout)
			t.Logf("(this can happen if you have a conflicting copy of the plugin in one of the global plugin search dirs)")
		}
	})

}

func TestInitProvidersCustomMethod(t *testing.T) {
	t.Parallel()

	// This test should not reach out to the network if it is behaving as
	// intended. If it _does_ try to access an upstream registry and encounter
	// an error doing so then that's a legitimate test failure that should be
	// fixed. (If it incorrectly reaches out anywhere then it's likely to be
	// to the host "example.com", which is the placeholder domain we use in
	// the test fixture.)

	for _, configFile := range []string{"cliconfig.tfrc", "cliconfig.tfrc.json"} {
		t.Run(configFile, func(t *testing.T) {
			fixturePath := filepath.Join("testdata", "custom-provider-install-method")
			tf := e2e.NewBinary(t, tofuBin, fixturePath)

			// Our fixture dir has a generic os_arch dir, which we need to customize
			// to the actual OS/arch where this test is running in order to get the
			// desired result.
			fixtMachineDir := tf.Path("fs-mirror/example.com/awesomecorp/happycloud/1.2.0/os_arch")
			wantMachineDir := tf.Path("fs-mirror/example.com/awesomecorp/happycloud/1.2.0/", fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH))
			err := os.Rename(fixtMachineDir, wantMachineDir)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			// We'll use a local CLI configuration file taken from our fixture
			// directory so we can force a custom installation method config.
			tf.AddEnv("TF_CLI_CONFIG_FILE=" + tf.Path(configFile))

			stdout, stderr, err := tf.Run("init")
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}

			if stderr != "" {
				t.Errorf("unexpected stderr output:\n%s", stderr)
			}

			if !strings.Contains(stdout, "OpenTofu has been successfully initialized!") {
				t.Errorf("success message is missing from output:\n%s", stdout)
			}

			if !strings.Contains(stdout, "- Installing example.com/awesomecorp/happycloud v1.2.0") {
				t.Errorf("provider download message is missing from output:\n%s", stdout)
			}
		})
	}
}

func TestInitProviders_pluginCache(t *testing.T) {
	t.Parallel()

	// This test reaches out to registry.opentofu.org to access plugin
	// metadata, and download the null plugin, though the template plugin
	// should come from local cache.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "plugin-cache")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// Our fixture dir has a generic os_arch dir, which we need to customize
	// to the actual OS/arch where this test is running in order to get the
	// desired result.
	fixtMachineDir := tf.Path("cache/registry.opentofu.org/hashicorp/template/2.1.0/os_arch")
	wantMachineDir := tf.Path("cache/registry.opentofu.org/hashicorp/template/2.1.0/", fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH))
	err := os.Rename(fixtMachineDir, wantMachineDir)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	extension := ""
	if runtime.GOOS == "windows" {
		extension = ".exe"

		// Fix EXE path
		target := path.Join(wantMachineDir, "terraform-provider-template_v2.1.0_x4")
		err := os.Rename(target, target+extension)
		if err != nil {
			t.Fatal(err)
		}

		// TODO add .exe entry to lockfile
		t.Skip()
	}

	cmd := tf.Cmd("init")

	// convert the slashes if building for windows.
	p := filepath.FromSlash("./cache")
	cmd.Env = append(cmd.Env, "TF_PLUGIN_CACHE_DIR="+p)
	err = cmd.Run()
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	path := filepath.FromSlash(fmt.Sprintf(".terraform/providers/registry.opentofu.org/hashicorp/template/2.1.0/%s_%s/terraform-provider-template_v2.1.0_x4", runtime.GOOS, runtime.GOARCH)) + extension
	content, err := tf.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read installed plugin from %s: %s", path, err)
	}
	if strings.TrimSpace(string(content)) != "this is not a real plugin" {
		t.Errorf("template plugin was not installed from local cache")
	}

	nullLinkPath := filepath.FromSlash(fmt.Sprintf(".terraform/providers/registry.opentofu.org/hashicorp/null/2.1.0/%s_%s/terraform-provider-null", runtime.GOOS, runtime.GOARCH)) + extension
	if !tf.FileExists(nullLinkPath) {
		t.Errorf("null plugin was not installed into %s", nullLinkPath)
	}

	nullCachePath := filepath.FromSlash(fmt.Sprintf("cache/registry.opentofu.org/hashicorp/null/2.1.0/%s_%s/terraform-provider-null", runtime.GOOS, runtime.GOARCH)) + extension
	if !tf.FileExists(nullCachePath) {
		t.Errorf("null plugin is not in cache after install. expected in: %s", nullCachePath)
	}
}

func TestInit_fromModule(t *testing.T) {
	t.Parallel()

	// This test reaches out to registry.opentofu.org and github.com to lookup
	// and fetch a module.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "empty")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	cmd := tf.Cmd("init", "-from-module=hashicorp/vault/aws")
	cmd.Stdin = nil
	cmd.Stderr = &bytes.Buffer{}

	err := cmd.Run()
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	stderr := cmd.Stderr.(*bytes.Buffer).String()
	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	content, err := tf.ReadFile("main.tf")
	if err != nil {
		t.Fatalf("failed to read main.tf: %s", err)
	}
	if !bytes.Contains(content, []byte("vault")) {
		t.Fatalf("main.tf doesn't appear to be a vault configuration: \n%s", content)
	}
}

func TestInitProviderNotFound(t *testing.T) {
	t.Parallel()

	// This test will reach out to registry.opentofu.org as one of the possible
	// installation locations for hashicorp/nonexist, which should not exist.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "provider-not-found")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	t.Run("registry provider not found", func(t *testing.T) {
		_, stderr, err := tf.Run("init", "-no-color")
		if err == nil {
			t.Fatal("expected error, got success")
		}

		oneLineStderr := strings.ReplaceAll(stderr, "\n", " ")
		if !strings.Contains(oneLineStderr, "provider registry registry.opentofu.org does not have a provider named registry.opentofu.org/hashicorp/nonexist") {
			t.Errorf("expected error message is missing from output:\n%s", stderr)
		}

		if !strings.Contains(oneLineStderr, "All modules should specify their required_providers") {
			t.Errorf("expected error message is missing from output:\n%s", stderr)
		}
	})

	t.Run("registry provider not found output in json format", func(t *testing.T) {
		stdout, _, err := tf.Run("init", "-no-color", "-json")
		if err == nil {
			t.Fatal("expected error, got success")
		}

		oneLineStdout := strings.ReplaceAll(stdout, "\n", " ")
		if !strings.Contains(oneLineStdout, `"diagnostic":{"severity":"error","summary":"Failed to query available provider packages","detail":"Could not retrieve the list of available versions for provider hashicorp/nonexist: provider registry registry.opentofu.org does not have a provider named registry.opentofu.org/hashicorp/nonexist\n\nAll modules should specify their required_providers so that external consumers will get the correct providers when using a module. To see which modules are currently depending on hashicorp/nonexist, run the following command:\n    tofu providers\n\nIf you believe this provider is missing from the registry, please submit a issue on the OpenTofu Registry https://github.com/opentofu/registry/issues/new/choose"},"type":"diagnostic"}`) {
			t.Errorf("expected error message is missing from output:\n%s", stdout)
		}
	})

	t.Run("local provider not found", func(t *testing.T) {
		// The -plugin-dir directory must exist for the provider installer to search it.
		pluginDir := tf.Path("empty-for-json")
		if err := os.Mkdir(pluginDir, os.ModePerm); err != nil {
			t.Fatal(err)
		}

		_, stderr, err := tf.Run("init", "-no-color", "-plugin-dir="+pluginDir)
		if err == nil {
			t.Fatal("expected error, got success")
		}

		if !strings.Contains(stderr, "provider registry.opentofu.org/hashicorp/nonexist was not\nfound in any of the search locations\n\n  - "+pluginDir) {
			t.Errorf("expected error message is missing from output:\n%s", stderr)
		}
	})

	t.Run("local provider not found output in json format", func(t *testing.T) {
		// The -plugin-dir directory must exist for the provider installer to search it.
		pluginDir := tf.Path("empty")
		if err := os.Mkdir(pluginDir, os.ModePerm); err != nil {
			t.Fatal(err)
		}

		stdout, _, err := tf.Run("init", "-no-color", "-plugin-dir="+pluginDir, "-json")
		if err == nil {
			t.Fatal("expected error, got success")
		}

		escapedPluginDir := escapeStringJSON(pluginDir)

		if !strings.Contains(stdout, `"diagnostic":{"severity":"error","summary":"Failed to query available provider packages","detail":"Could not retrieve the list of available versions for provider hashicorp/nonexist: provider registry.opentofu.org/hashicorp/nonexist was not found in any of the search locations\n\n  - `+escapedPluginDir+`"},"type":"diagnostic"}`) {
			t.Errorf("expected error message is missing from output (pluginDir = '%s'):\n%s", escapedPluginDir, stdout)
		}
	})

	t.Run("special characters enabled", func(t *testing.T) {
		_, stderr, err := tf.Run("init")
		if err == nil {
			t.Fatal("expected error, got success")
		}

		expectedErr := `╷
│ Error: Failed to query available provider packages
│` + ` ` + `
│ Could not retrieve the list of available versions for provider
│ hashicorp/nonexist: provider registry registry.opentofu.org does not have a
│ provider named registry.opentofu.org/hashicorp/nonexist
│ 
│ All modules should specify their required_providers so that external
│ consumers will get the correct providers when using a module. To see which
│ modules are currently depending on hashicorp/nonexist, run the following
│ command:
│     tofu providers
│ 
│ If you believe this provider is missing from the registry, please submit a
│ issue on the OpenTofu Registry
│ https://github.com/opentofu/registry/issues/new/choose
╵

`
		if stripAnsi(stderr) != expectedErr {
			t.Errorf("wrong output:\n%s", cmp.Diff(stripAnsi(stderr), expectedErr))
		}
	})

	t.Run("implicit provider resource and data not found", func(t *testing.T) {
		implicitFixturePath := filepath.Join("testdata", "provider-implicit-ref-not-found/implicit-by-resource-and-data")
		tf := e2e.NewBinary(t, tofuBin, implicitFixturePath)
		stdout, _, err := tf.Run("init")
		if err == nil {
			t.Fatal("expected error, got success")
		}

		// Testing that the warn wrote to the user is containing the resource address from where the provider
		// was registered to be downloaded
		expectedContentInOutput := []string{
			`(and one more similar warning elsewhere)`,
			`
╷
│ Warning: Automatically-inferred provider dependency
│ 
│   on main.tf line 2:
│    2: resource "nonexistingProv_res" "test1" {
│ 
│ Due to the prefix of the resource type name OpenTofu guessed that you
│ intended to associate nonexistingProv_res.test1 with a provider whose local
│ name is "nonexistingprov", but that name is not declared in this module's
│ required_providers block. OpenTofu therefore guessed that you intended to
│ use hashicorp/nonexistingprov, but that provider does not exist.
│ 
│ Make at least one of the following changes to tell OpenTofu which provider
│ to use:
│ 
│ - Add a declaration for local name "nonexistingprov" to this module's
│ required_providers block, specifying the full source address for the
│ provider you intended to use.
│ - Verify that "nonexistingProv_res" is the correct resource type name to
│ use. Did you omit a prefix which would imply the correct provider?
│ - Use a "provider" argument within this resource block to override
│ OpenTofu's automatic selection of the local name "nonexistingprov".
│`}
		for _, expectedOutput := range expectedContentInOutput {
			if cleanOut := strings.TrimSpace(stripAnsi(stdout)); !strings.Contains(cleanOut, expectedOutput) {
				t.Errorf("wrong output.\n\toutput:\n%s\n\n\tdoes not contain:\n%s", cleanOut, expectedOutput)
			}
		}
	})

	t.Run("resource pointing to a not configured provider does not warn on implicit reference", func(t *testing.T) {
		implicitFixturePath := filepath.Join("testdata", "provider-implicit-ref-not-found/resource-with-provider-attribute")
		tf := e2e.NewBinary(t, tofuBin, implicitFixturePath)
		stdout, _, err := tf.Run("init")
		if err == nil {
			t.Fatal("expected error, got success")
		}

		// Ensure that the output does not contain the warning since the resource is pointing already to a specific
		// provider (even though it is misspelled)
		expectedOutput := `Initializing the backend...

Initializing provider plugins...
- Finding latest version of hashicorp/asw...`
		if cleanOut := strings.TrimSpace(stripAnsi(stdout)); cleanOut != expectedOutput {
			t.Errorf("wrong output:\n%s", cmp.Diff(cleanOut, expectedOutput))
		}
	})
}

// The following test is temporarily removed until the OpenTofu registry returns a deprecation warning
// https://github.com/opentofu/registry/issues/108
//func TestInitProviderWarnings(t *testing.T) {
//	t.Parallel()
//
//  // This test will reach out to registry.terraform.io as one of the possible
//  // installation locations for hashicorp/terraform, which is an archived package that is no longer needed.
//	skipIfCannotAccessNetwork(t)
//
//	fixturePath := filepath.Join("testdata", "provider-warnings")
//	tf := e2e.NewBinary(t, tofuBin, fixturePath)
//
//	stdout, _, err := tf.Run("init")
//	if err == nil {
//		t.Fatal("expected error, got success")
//	}
//
//	if !strings.Contains(stdout, "This provider is archived and no longer needed.") {
//		t.Errorf("expected warning message is missing from output:\n%s", stdout)
//	}
//
//}

func escapeStringJSON(v string) string {
	b := &strings.Builder{}

	enc := json.NewEncoder(b)

	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		panic("failed to escapeStringJSON: " + v)
	}

	marshaledV := b.String()

	// shouldn't happen
	if len(marshaledV) < 2 {
		return string(marshaledV)
	}

	return string(marshaledV[1 : len(marshaledV)-2])
}
