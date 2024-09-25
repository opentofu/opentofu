// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/e2e"
)

// The tests in this file are for the "tofu providers mirror" command,
// which is tested in an e2etest mode rather than a unit test mode because it
// interacts directly with OpenTofu Registry and the full details of that are
// tricky to mock. Such a mock is _possible_, but we're using e2etest as a
// compromise for now to keep these tests relatively simple.

func TestOpenTofuProvidersMirror(t *testing.T) {
	testOpenTofuProvidersMirror(t, "tofu-providers-mirror")
}

func TestOpenTofuProvidersMirrorWithLockFile(t *testing.T) {
	testOpenTofuProvidersMirror(t, "tofu-providers-mirror-with-lock-file")
}

func testOpenTofuProvidersMirror(t *testing.T, fixture string) {
	// This test reaches out to registry.opentofu.org to download the
	// template and null providers, so it can only run if network access is
	// allowed.
	skipIfCannotAccessNetwork(t)

	outputDir := t.TempDir()
	t.Logf("creating mirror directory in %s", outputDir)

	fixturePath := filepath.Join("testdata", fixture)
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	stdout, stderr, err := tf.Run("providers", "mirror", "-platform=linux_amd64", "-platform=windows_386", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %s\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	// The test fixture includes exact version constraints for the two
	// providers it depends on so that the following should remain stable.
	// In the (unlikely) event that these particular versions of these
	// providers are removed from the registry, this test will start to fail.
	want := []string{
		"registry.opentofu.org/hashicorp/null/2.1.0.json",
		"registry.opentofu.org/hashicorp/null/index.json",
		"registry.opentofu.org/hashicorp/null/terraform-provider-null_2.1.0_linux_amd64.zip",
		"registry.opentofu.org/hashicorp/null/terraform-provider-null_2.1.0_windows_386.zip",
		"registry.opentofu.org/hashicorp/template/2.1.1.json",
		"registry.opentofu.org/hashicorp/template/index.json",
		"registry.opentofu.org/hashicorp/template/terraform-provider-template_2.1.1_linux_amd64.zip",
		"registry.opentofu.org/hashicorp/template/terraform-provider-template_2.1.1_windows_386.zip",
	}
	var got []string
	walkErr := filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil // we only care about leaf files for this test
		}
		relPath, err := filepath.Rel(outputDir, path)
		if err != nil {
			return err
		}
		got = append(got, filepath.ToSlash(relPath))
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	sort.Strings(got)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected files in result\n%s", diff)
	}
}

// this test is based on testOpenTofuProvidersMirror above.
func TestOpenTofuProvidersMirrorBadLockfile(t *testing.T) {
	// This test reaches out to registry.opentofu.org to download the
	// template and null providers, so it can only run if network access is
	// allowed.
	skipIfCannotAccessNetwork(t)

	outputDir := t.TempDir()
	t.Logf("creating mirror directory in %s", outputDir)

	fixturePath := filepath.Join("testdata", "tofu-providers-mirror-with-bad-lock-file")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	stdout, stderr, err := tf.Run("providers", "mirror", outputDir)
	if err == nil {
		t.Fatalf("expected error: %s\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	var want []string
	var got []string
	walkErr := filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil // we only care about leaf files for this test
		}
		relPath, err := filepath.Rel(outputDir, path)
		if err != nil {
			return err
		}
		got = append(got, filepath.ToSlash(relPath))
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	sort.Strings(got)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected files in result\n%s", diff)
	}
}
