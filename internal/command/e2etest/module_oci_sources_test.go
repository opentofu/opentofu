// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/opentofu/opentofu/internal/command/e2etest/fakeocireg"
	"github.com/opentofu/opentofu/internal/e2e"
)

// This file contains a small amount of end-to-end testing that's primarily intended
// to test the "dependency wiring" code in package main, to ensure that configuring
// an OCI Registry-based module source address does actually cause the system to
// successfully use that source.
//
// Add new tests here only as a last resort! In particular, if you are interested in
// testing behaviors that live in package getmodules, package cliconfig, or
// package ociauthconfig then it's better to add unit tests there rather than
// more end-to-end tests here, because end-to-end tests are harder to maintain and
// harder to debug when they fail.

// TestModuleOCISources is an end-to-end test that runs "tofu init" against a root
// module that requests another module using the "oci:" source address scheme, which
// should therefore successfully install the module package using the OCI Distribution
// protocol.
//
// (This is therefore not quite as "end-to-end" as most of our tests in this package,
// but it does at least test the part of the behavior that lives inside OpenTofu
// end-to-end, even though the OCI registry server is faked out.)
func TestModuleOCISources(t *testing.T) {
	// Our typical rule for external service access in e2etests is that it's okay
	// to access servers run by the OpenTofu project when TF_ACC=1 is set in the
	// environment. However, the OpenTofu project does not currently run an
	// OCI registry and we don't want to rely on anything we can't influence the
	// reliability of, so for this test we rely on a local fake registry implementation
	// based on some OCI layouts provided as test fixtures.
	//
	// This fake server uses a locally-generated TLS certificate and so we'll need to
	// override the trusted certs for the child process so it can be used successfully.
	// The Go toolchain only allows doing that by environment variable on Unix systems
	// other than macOS, and so in practice this test is only suitable for a limited set
	// of platforms. The main OCI registry client code is not platform-specific anyway, and
	// this is an e2etest mainly just because it's an effective way to test "package main"
	// behavior rather than for the cross-platform testing capability, so while this is
	// not ideal it's a reasonable compromise to keep this test relatively self-contained.
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("this test is suitable only for linux_amd64 and linux_arm64")
	}

	registryServer, err := fakeocireg.NewServer(t.Context(), map[string]string{
		"foo": filepath.Join("testdata", "oci-module-source", "fake-registry", "foo"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer registryServer.Close()
	registryAddr := registryServer.Listener.Addr().String()

	// We need to pass the fake registry server's certificate to the child process
	// using a temporary file on disk.
	tempDir := t.TempDir()
	certFile := filepath.Join(tempDir, "testserver.crt")
	err = writeCertificatePEMFile(certFile, registryServer.Certificate())
	if err != nil {
		t.Fatalf("failed to create temporary certificate file: %s", err)
	}
	t.Logf("using temporary TLS certificate file at %s", certFile)

	// The following should all successfully install the same fake module package.
	validSourceAddresses := []string{
		"oci://" + registryAddr + "/foo",
		"oci://" + registryAddr + "/foo?tag=latest",
		"oci://" + registryAddr + "/foo?digest=sha256:a47afe6606ae1d53305461b023c193c50af457259e40dc45bd068f62ef408034",
	}
	for _, sourceAddr := range validSourceAddresses {
		t.Run(sourceAddr, func(t *testing.T) {
			t.Logf("testing with source = %q", sourceAddr)
			// We need a root module that requests the remote module package
			// we're intending to fetch for testing.
			rootModDir := t.TempDir()
			err := os.WriteFile(
				filepath.Join(rootModDir, "test.tf"),
				[]byte(`
					module "test" {
						source = "`+sourceAddr+`"
					}
				`),
				os.ModePerm,
			)
			if err != nil {
				t.Fatalf("failed to create test configuration: %s", err)
			}

			tf := e2e.NewBinary(t, tofuBin, rootModDir)
			t.Logf("running 'tofu init' in temporary directory %q", tf.Path())
			tf.AddEnv("SSL_CERT_FILE=" + certFile)
			_, stderr, err := tf.Run("init", "-backend=false")
			if err != nil {
				t.Logf("tofu init stderr:\n%s", stderr)
				t.Fatalf("failed to run tofu init: %s", err)
			}

			// If installation succeeded then we should have a copy of the
			// module package in the cache under the data directory. The
			// exact layout of this is an implementation detail rather than
			// a compatibility constraint, so it's okay to update this
			// to use a different location if future work changes how we
			// structure the cache of remote module source packages.
			//wantFilename := filepath.Join(dataDir, "modules", "test", "fake_module_package.tofu")
			wantFilename := tf.Path(".terraform", "modules", "test", "fake_module_package.tofu")
			info, err := os.Lstat(wantFilename)
			if err != nil {
				t.Fatalf("can't stat %q after apparently-successful installation: %s", wantFilename, err)
			}
			if info.IsDir() {
				t.Errorf("installed package contains %q as directory, but should be a file", wantFilename)
			}
		})
	}
}
