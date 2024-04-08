package e2etest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

func TestSimpleFunction(t *testing.T) {
	// This test reaches out to registry.opentofu.org to download the
	// helper function provider, so it can only run if network access is allowed
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "functions")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// tofu init
	_, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	_, stderr, err = tf.Run("plan", "-out=fnplan")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	plan, err := tf.Plan("fnplan")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	if len(plan.Changes.Outputs) != 1 {
		t.Fatalf("expected 1 outputs, got %d", len(plan.Changes.Outputs))
	}
	for _, out := range plan.Changes.Outputs {
		if !strings.Contains(string(out.After), "Hello Functions") {
			t.Fatalf("unexpected plan output: %s", string(out.After))
		}
	}

}
