// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

// TestProviderNetworkMirrorRetries checks that the retries configuration for downloading the
// provider from a network mirror source is handled correctly alone and also together with
// the TF_PROVIDER_DOWNLOAD_RETRY env var.
//
// This is testing the same thing as TestInitProviderSourceForCLIConfigLocationWithRetries but
// it's an e2e test instead because there is no way to inject the TLS certificate properly into
// the underlying implementation that talks with a remote network mirror source.
func TestProviderNetworkMirrorRetries(t *testing.T) {
	// Our typical rule for external service access in e2etests is that it's okay
	// to access servers run by the OpenTofu project when TF_ACC=1 is set in the
	// environment. However, this particular test is checking a network mirror source
	// that needs to be configured with a TLS certificate and that certificate to be given
	// in the `tofu` child process env vars to be used for talking with the server.
	// The entirety of this process can be done without actual network access so it can run
	// without the TF_ACC=1.
	//
	// We restrict this for only linux_amd64 to make it easier to maintain since the whole purpose
	// of the test has nothing to do with the actual platform it runs on.
	//
	// The fake server uses a locally-generated TLS certificate and so we'll need to
	// override the trusted certs for the child process so it can be used successfully.
	// The Go toolchain only allows doing that by environment variable on Unix systems
	// other than macOS.
	// Additionally, for ease of maintenance, the stubbed data inside this test is only
	// for linux_amd64 so we cannot run this for other platforms that still support the
	// previously mentioned limitation.
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("this test is suitable only for linux_amd64")
	}
	networkMirrorHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/example.com/test/test/index.json":
			w.Header().Add("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"versions":{"0.0.1":{}}}`))
			return
		case "/example.com/test/test/0.0.1.json":
			w.Header().Add("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"archives": {"linux_amd64": {"url": "terraform-provider-test_0.0.1_linux_amd64.zip","hashes": []}}}`))
			return
		case "/example.com/test/test/terraform-provider-test_0.0.1_linux_amd64.zip":
			w.WriteHeader(http.StatusInternalServerError)
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cases := map[string]struct {
		tofurcRetriesConfigEntry string
		envVars                  map[string]string
		expectedErrMsg           string
	}{
		"no tofurc.network_mirror.download_retry_count, no TF_PROVIDER_DOWNLOAD_RETRY, default TF_PROVIDER_DOWNLOAD_RETRY used": {
			tofurcRetriesConfigEntry: "",
			envVars:                  nil,
			expectedErrMsg:           "/example.com/test/test/terraform-provider-test_0.0.1_linux_amd64.zip giving up after 3 attempt(s)",
		},
		"no tofurc.network_mirror.download_retry_count, TF_PROVIDER_DOWNLOAD_RETRY defined, TF_PROVIDER_DOWNLOAD_RETRY used": {
			tofurcRetriesConfigEntry: "",
			envVars: map[string]string{
				"TF_PROVIDER_DOWNLOAD_RETRY": "1",
			},
			expectedErrMsg: "/example.com/test/test/terraform-provider-test_0.0.1_linux_amd64.zip giving up after 2 attempt(s)",
		},
		"defined tofurc.network_mirror.download_retry_count as 0, no TF_PROVIDER_DOWNLOAD_RETRY, tofurc used": {
			tofurcRetriesConfigEntry: "download_retry_count = 0",
			envVars:                  nil,
			expectedErrMsg:           "/example.com/test/test/terraform-provider-test_0.0.1_linux_amd64.zip giving up after 1 attempt(s)",
		},
		"defined tofurc.network_mirror.download_retry_count as 1, no TF_PROVIDER_DOWNLOAD_RETRY, tofurc used": {
			tofurcRetriesConfigEntry: "download_retry_count = 1",
			envVars:                  nil,
			expectedErrMsg:           "/example.com/test/test/terraform-provider-test_0.0.1_linux_amd64.zip giving up after 2 attempt(s)",
		},
		"defined tofurc.network_mirror.download_retry_count as 1, TF_PROVIDER_DOWNLOAD_RETRY defined as 2, tofurc used": {
			tofurcRetriesConfigEntry: "download_retry_count = 1",
			envVars: map[string]string{
				"TF_PROVIDER_DOWNLOAD_RETRY": "2",
			},
			expectedErrMsg: "/example.com/test/test/terraform-provider-test_0.0.1_linux_amd64.zip giving up after 2 attempt(s)",
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewTLSServer(networkMirrorHandler)
			defer server.Close()
			registryAddr := server.URL

			// We need to pass both our fake registry server's certificate and the CLI config
			// for interacting with it to the child process using some temporary files on disk.
			tempDir := t.TempDir()
			certFile := filepath.Join(tempDir, "testserver.crt")
			if err := writeCertificatePEMFile(certFile, server.Certificate()); err != nil {
				t.Fatalf("failed to create temporary certificate file: %s", err)
			}
			cliConfigFile := filepath.Join(tempDir, "cliconfig.tfrc")
			cliConfigSrc := fmt.Sprintf(`
		provider_installation {
			network_mirror {
                url = "%s"
				%s
			}
		}
	`, registryAddr, tt.tofurcRetriesConfigEntry)
			if err := os.WriteFile(cliConfigFile, []byte(cliConfigSrc), os.ModePerm); err != nil {
				t.Fatalf("failed to create temporary CLI configuration file: %s", err)
			}
			dataDir := filepath.Join(tempDir, ".terraform")

			tf := e2e.NewBinary(t, tofuBin, "testdata/provider-network-mirror")
			tf.AddEnv("SSL_CERT_FILE=" + certFile)
			tf.AddEnv("TF_CLI_CONFIG_FILE=" + cliConfigFile)
			tf.AddEnv("TF_DATA_DIR=" + dataDir)
			for k, v := range tt.envVars {
				tf.AddEnv(fmt.Sprintf("%s=%s", k, v))
			}
			_, stderr, err := tf.Run("init", "-backend=false")
			if err == nil {
				t.Fatalf("expected `tofu init` to fail but got no error")
			}
			t.Logf("stderr:\n%s", stderr)
			cleanStderr := SanitizeStderr(stderr)
			if contains := tt.expectedErrMsg; !strings.Contains(cleanStderr, contains) {
				t.Fatalf("expected the error from the installation to contain %q but it doesn't", contains)
			}
		})
	}
}
