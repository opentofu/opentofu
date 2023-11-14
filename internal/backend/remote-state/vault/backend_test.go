package vault

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/serf/testutil/retry"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/pkg/errors"
)

const vaultApiAddr = "http://127.0.0.1:8200"

func TestBackend_impl(t *testing.T) {
	var _ backend.Backend = new(Backend)
}

func newVaultTestServer(t *testing.T) *TestServer {
	if os.Getenv("TF_ACC") == "" && os.Getenv("TF_VAULT_TEST") == "" {
		t.Skipf("vault server tests require setting TF_ACC or TF_VAULT_TEST")
	}

	srv, err := NewTestServerConfigT(t)
	if err != nil {
		t.Fatalf("failed to create vault test server: %s", err)
	}

	return srv
}

func TestBackend(t *testing.T) {
	srv := newVaultTestServer(t)
	defer srv.Stop()

	path := fmt.Sprintf("tf-unit/%s", time.Now().String())

	// Get the backend. We need two to test locking.
	b1 := backend.TestBackendConfig(t, New(), backend.TestWrapConfig(map[string]interface{}{
		"address": srv.HTTPAddr,
		"path":    path,
	}))

	b2 := backend.TestBackendConfig(t, New(), backend.TestWrapConfig(map[string]interface{}{
		"address": srv.HTTPAddr,
		"path":    path,
	}))

	// Test
	backend.TestBackendStates(t, b1)
	backend.TestBackendStateLocks(t, b1, b2)
}

func TestBackend_lockDisabled(t *testing.T) {
	srv := newVaultTestServer(t)
	defer srv.Stop()

	path := fmt.Sprintf("tf-unit/%s", time.Now().String())

	// Get the backend. We need two to test locking.
	b1 := backend.TestBackendConfig(t, New(), backend.TestWrapConfig(map[string]interface{}{
		"address": srv.HTTPAddr,
		"path":    path,
		"lock":    false,
	}))

	b2 := backend.TestBackendConfig(t, New(), backend.TestWrapConfig(map[string]interface{}{
		"address": srv.HTTPAddr,
		"path":    path + "different", // Diff so locking test would fail if it was locking
		"lock":    false,
	}))

	// Test
	backend.TestBackendStates(t, b1)
	backend.TestBackendStateLocks(t, b1, b2)
}

func TestBackend_gzip(t *testing.T) {
	srv := newVaultTestServer(t)
	defer srv.Stop()

	// Get the backend
	b := backend.TestBackendConfig(t, New(), backend.TestWrapConfig(map[string]interface{}{
		"address": srv.HTTPAddr,
		"path":    fmt.Sprintf("tf-unit/%s", time.Now().String()),
		"gzip":    true,
	}))

	// Test
	backend.TestBackendStates(t, b)
}

// TestServer is the main server wrapper struct.
type TestServer struct {
	cmd *exec.Cmd

	HTTPAddr   string
	HTTPClient *http.Client
}

// NewTestServerConfigT creates a new TestServer. If there is an error
// configuring or starting the server, the server will NOT be running when the
// function returns (thus you do not need to stop it).
func NewTestServerConfigT(t *testing.T) (*TestServer, error) {
	path, err := exec.LookPath("vault")
	if err != nil || path == "" {
		return nil, fmt.Errorf("vault not found on $PATH - download and install " +
			"vault or skip this test")
	}

	logBuffer := testutil.NewLogBuffer(t)

	if !flag.Parsed() {
		flag.Parse()
	}

	if !testing.Verbose() {
		logBuffer = io.Discard
	}

	rootToken, err := uuid.GenerateUUID()
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate UUID")
	}

	os.Setenv("VAULT_TOKEN", rootToken)
	os.Setenv("VAULT_API_ADDR", vaultApiAddr)

	// Start the server
	args := []string{"server", "-dev", "-dev-root-token-id=" + rootToken, "-log-level=warn"}
	cmd := exec.Command("vault", args...)
	cmd.Stdout = logBuffer
	cmd.Stderr = logBuffer
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed starting command")
	}

	server := &TestServer{
		cmd: cmd,

		HTTPAddr:   vaultApiAddr,
		HTTPClient: cleanhttp.DefaultClient(),
	}

	// Wait for the server to be ready
	if err := server.waitForAPI(); err != nil {
		if err := server.Stop(); err != nil {
			t.Logf("server stop failed with: %v", err)
		}
		return nil, err
	}

	return server, nil
}

// Stop stops the test Vault server.
func (s *TestServer) Stop() error {
	// There was no process
	if s.cmd == nil {
		return nil
	}

	if s.cmd.Process != nil {
		if runtime.GOOS == "windows" {
			if err := s.cmd.Process.Kill(); err != nil {
				return errors.Wrap(err, "failed to kill vault server")
			}
		} else { // interrupt is not supported in windows
			if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
				return errors.Wrap(err, "failed to kill vault server")
			}
		}
	}

	waitDone := make(chan error)
	go func() {
		waitDone <- s.cmd.Wait()
		close(waitDone)
	}()

	// wait for the process to exit to be sure that the data dir can be
	// deleted on all platforms.
	select {
	case err := <-waitDone:
		return err
	case <-time.After(10 * time.Second):
		s.cmd.Process.Signal(syscall.SIGABRT)
		s.cmd.Wait()
		return fmt.Errorf("timeout waiting for server to stop gracefully")
	}
}

func (s *TestServer) waitForAPI() error {
	var failed bool

	// This retry replicates the logic of retry.Run to allow for nested retries.
	// By returning an error we can wrap TestServer creation with retry.Run
	// in makeClientWithConfig.
	timer := retry.TwoSeconds()
	deadline := time.Now().Add(timer.Timeout)
	for !time.Now().After(deadline) {
		time.Sleep(timer.Wait)

		req, err := http.NewRequest("GET", s.HTTPAddr+"/v1/sys/health", nil)
		if err != nil {
			failed = true
			continue
		}

		resp, err := s.HTTPClient.Do(req)
		if err != nil {
			failed = true
			continue
		}
		resp.Body.Close()

		failed = false
	}
	if failed {
		return fmt.Errorf("api unavailable")
	}
	return nil
}
