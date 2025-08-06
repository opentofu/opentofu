package e2etest

import (
	"os"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

// TestEphemeralErrors_variables checks common errors when ephemeral variables
// are referenced in contexts where it's not allowed to.
func TestEphemeralErrors_variables(t *testing.T) {
	tf := e2e.NewBinary(t, tofuBin, "testdata/ephemeral-errors/variables")
	buildSimpleProvider(t, "6", tf.WorkDir(), "simple")
	with := func(path string, fn func()) {
		src := tf.Path(path + ".disabled")
		dst := tf.Path(path)

		err := os.Rename(src, dst)
		if err != nil {
			t.Fatalf("%s", err.Error())
		}

		fn()

		err = os.Rename(dst, src)
		if err != nil {
			t.Fatalf("%s", err.Error())
		}
	}

	sout, serr, err := tf.Run("init", "-plugin-dir=cache")
	if err != nil {
		t.Fatalf("unable to init: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
	}
	{ // run it without being referenced
		sout, serr, err := tf.Run("apply", "-var", "in=test")
		if serr != "" {
			t.Fatalf("expected no stderr:\n%s", serr)
		}
		if err != nil {
			t.Fatalf("expected no err: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
	}
	{ // run with validation failures
		sout, serr, err := tf.Run("apply", "-var", "in=test", "-var", "in2=fail_on_this_value")
		if err == nil || !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("unexpected err: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
		sanitized := SanitizeStderr(serr)
		if !strings.Contains(sanitized, `The error expression used to explain this condition refers to ephemeral values. OpenTofu will not display the resulting message.  You can correct this by removing references to ephemeral values.`) {
			t.Errorf("unexpected stderr: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
			t.Logf("sanitized serr: %s", sanitized)
		}
	}
	with("in-resources-regular-fields.tf", func() {
		sout, serr, err := tf.Run("apply", "-var", "in=test")
		if err == nil || !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("unexpected err: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
		sanitized := SanitizeStderr(serr)
		if !strings.Contains(sanitized, `Error: Ephemeral value used in non-ephemeral context    with simple_resource.test_res`) {
			t.Errorf("unexpected stderr: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
			t.Logf("sanitized serr: %s", sanitized)
		}
	})
	with("in-data-source.tf", func() {
		sout, serr, err := tf.Run("apply", "-var", "in=test")
		if err == nil || !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("unexpected err: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
		sanitized := SanitizeStderr(serr)
		if !strings.Contains(sanitized, `Ephemeral value used in non-ephemeral context    with data.simple_resource.test_data1`) {
			t.Errorf("unexpected stderr: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
			t.Logf("sanitized serr: %s", sanitized)
		}
	})
	with("in-non-ephemeral-mod-variable.tf", func() {
		// NOTE: Need to reinit to install the module
		sout, serr, err := tf.Run("init", "-plugin-dir=cache")
		if err != nil {
			t.Fatalf("unable to init: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}

		sout, serr, err = tf.Run("apply", "-var", "in=test")
		if err == nil || !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("unexpected err: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
		sanitized := SanitizeStderr(serr)
		if !strings.Contains(sanitized, `Variable does not allow ephemeral value    on in-non-ephemeral-mod-variable.tf line 5, in module "test"`) {
			t.Errorf("unexpected stderr: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
			t.Logf("sanitized serr: %s", sanitized)
		}
	})
	with("in-non-ephemeral-output.tf", func() {
		sout, serr, err := tf.Run("apply", "-var", "in=test")
		if err == nil || !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("unexpected err: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
		sanitized := SanitizeStderr(serr)
		if !strings.Contains(sanitized, `Output does not allow ephemeral value    on in-non-ephemeral-output.tf`) {
			t.Errorf("unexpected stderr: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
			t.Logf("sanitized serr: %s", sanitized)
		}
	})
}
