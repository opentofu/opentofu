// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/e2etest/fakeocireg"
	"github.com/opentofu/opentofu/internal/e2e"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/providercache"
)

// This file contains a small amount of end-to-end testing that's primarily intended
// to test the "dependency wiring" code in package main, to ensure that configuring
// an OCI mirror source in the CLI configuration does actually cause the system to
// use that source.
//
// Add new tests here only as a last resort! In particular, if you are interested in
// testing behaviors that live in package getproviders, package cliconfig, or
// package ociauthconfig then it's better to add unit tests there rather than
// more end-to-end tests here, because end-to-end tests are harder to maintain and
// harder to debug when they fail.

// TestProviderOCIMirrors is an end-to-end test that runs "tofu init" with the CLI
// configuration specifying an oci_mirror installation source, which should therefore
// successfully install some providers from a fake OCI registry that runs inside this
// test program.
//
// (This is therefore not quite as "end-to-end" as most of our tests in this package,
// but it does at least test the part of the behavior that lives inside OpenTofu
// end-to-end, even though the OCI registry server is faked out.)
func TestProviderOCIMirrors(t *testing.T) {
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
	// other than macOS, and our test fixture data only contains providers that claim
	// to be for Linux, macOS, and Windows on a small number of architectures, and so
	// in practice this test is only suitable for the following platforms. The main
	// OCI registry client code is not platform-specific anyway, and this is an e2etest
	// mainly just because it's an effective way to test "package main" behavior rather
	// than for the cross-platform testing capability, so while this is not ideal it's
	// a reasonable compromise to keep this test relatively self-contained.
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("this test is suitable only for linux_amd64 and linux_arm64")
	}

	registryServer, err := fakeocireg.NewServer(t.Context(), map[string]string{
		"foo": filepath.Join("testdata", "oci-provider-mirror", "fake-registry", "foo"),
		"bar": filepath.Join("testdata", "oci-provider-mirror", "fake-registry", "bar"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer registryServer.Close()
	registryAddr := registryServer.Listener.Addr().String()

	// We need to pass both our fake registry server's certificate and the CLI config
	// for interacting with it to the child process using some temporary files on disk.
	tempDir := t.TempDir()
	certFile := filepath.Join(tempDir, "testserver.crt")
	err = writeCertificatePEMFile(certFile, registryServer.Certificate())
	if err != nil {
		t.Fatalf("failed to create temporary certificate file: %s", err)
	}
	cliConfigFile := filepath.Join(tempDir, "cliconfig.tfrc")
	cliConfigSrc := fmt.Sprintf(`
		provider_installation {
			oci_mirror {
				repository_template = "%s/${type}"
				include             = ["example.com/test/*"]
			}
		}
	`, registryAddr)
	t.Logf("cli config:\n%s", cliConfigSrc)
	if err != nil {
		t.Fatalf("failed to marshal temporary CLI configuration: %s", err)
	}
	err = os.WriteFile(cliConfigFile, []byte(cliConfigSrc), os.ModePerm)
	if err != nil {
		t.Fatalf("failed to create temporary CLI configuration file: %s", err)
	}
	dataDir := filepath.Join(tempDir, ".terraform")

	tf := e2e.NewBinary(t, tofuBin, "testdata/oci-provider-mirror")
	tf.AddEnv("SSL_CERT_FILE=" + certFile)
	tf.AddEnv("TF_CLI_CONFIG_FILE=" + cliConfigFile)
	tf.AddEnv("TF_DATA_DIR=" + dataDir)
	_, stderr, err := tf.Run("init", "-backend=false")
	if err != nil {
		t.Logf("tofu init stderr:\n%s", stderr)
		t.Fatalf("failed to run tofu init: %s", err)
	}

	// If installation succeeded then we should now be able to successfully interact
	// with the provider cache directory. (The location of this relative to the
	// data dir is not actually considered a compatibility constraint, so if we
	// intentionally change that for some reason in a future version then it's okay
	// to update this providerCachePath definition to make the test pass again.)
	providerCachePath := filepath.Join(dataDir, "providers")
	providerCacheDir := providercache.NewDir(providerCachePath)
	gotPackages := providerCacheDir.AllAvailablePackages()
	fooProvider := addrs.MustParseProviderSourceString("example.com/test/foo")
	barProvider := addrs.MustParseProviderSourceString("example.com/test/bar")
	wantPackages := map[addrs.Provider][]providercache.CachedProvider{
		fooProvider: {
			{
				Provider:   fooProvider,
				Version:    getproviders.MustParseVersion("1.0.0"),
				PackageDir: filepath.Join(providerCachePath, "example.com", "test", "foo", "1.0.0", getproviders.CurrentPlatform.String()),
			},
		},
		barProvider: {
			{
				Provider:   barProvider,
				Version:    getproviders.MustParseVersion("1.0.0"),
				PackageDir: filepath.Join(providerCachePath, "example.com", "test", "bar", "1.0.0", getproviders.CurrentPlatform.String()),
				// If this fails with version 2.0.0 instead then that suggests
				// the version constraints in the configuration weren't honored.
			},
		},
	}
	if diff := cmp.Diff(wantPackages, gotPackages); diff != "" {
		t.Error("wrong packages in cache directory\n" + diff)
	}
}

// writeCertificatePEMFile is a helper that writes a PEM-formatted copy of the
// given certificate to the given file, so that it can be used with the
// SSL_CERT_FILE environment variable to encourage a child process to trust it.
func writeCertificatePEMFile(filename string, cert *x509.Certificate) error {
	src := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	return os.WriteFile(filename, src, os.ModePerm)
}
