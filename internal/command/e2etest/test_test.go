package e2etest

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opentofu/opentofu/internal/e2e"
)

func TestMultipleRunBlocks(t *testing.T) {
	timeout := time.After(3 * time.Second)
	type testResult struct {
		stdout string
		stderr string
		err    error
	}
	done := make(chan *testResult)

	go func() {
		fixturePath := filepath.Join("testdata", "multiple-run-blocks")
		tf := e2e.NewBinary(t, tofuBin, fixturePath)
		stdout, stderr, err := tf.Run("test")
		done <- &testResult{
			stdout: stdout,
			stderr: stderr,
			err:    err,
		}
	}()

	select {
	case <-timeout:
		t.Fatal("timed out")
	case result := <-done:
		if result.err != nil {
			t.Errorf("unexpected error: %s", result.err)
		}

		if result.stderr != "" {
			t.Errorf("unexpected stderr output:\n%s", result.stderr)
		}

		if !strings.Contains(result.stdout, "30 passed") {
			t.Errorf("success message is missing from output:\n%s", result.stdout)
		}
	}
}
