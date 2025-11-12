// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"os"
	"path/filepath"
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
		if !strings.Contains(sanitized, `The error expression used to explain this condition refers to ephemeral values. OpenTofu will not display the resulting message.  You can correct this by removing references to ephemeral values or by utilizing the builtin ephemeralasnull() function.`) {
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

func TestEphemeralErrors_outputs(t *testing.T) {
	tf := e2e.NewBinary(t, tofuBin, "testdata/ephemeral-errors/outputs")
	buildSimpleProvider(t, "6", tf.WorkDir(), "simple")
	with := func(path string, fn func()) {
		src := tf.Path(path + ".disabled")
		dst := tf.Path(path)
		tf.WorkDir()

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

	tofuInit := func() {
		sout, serr, err := tf.Run("init", "-plugin-dir=cache")
		if err != nil {
			t.Fatalf("unable to init: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
	}

	with("ephemeral_output_in_root_module.tf", func() {
		sout, serr, err := tf.Run("apply")
		if err == nil || !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("unexpected err: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
		sanitized := SanitizeStderr(serr)
		if !strings.Contains(sanitized, `Error: Invalid output configuration    on ephemeral_output_in_root_module.tf`) ||
			!strings.Contains(sanitized, `Root modules are not allowed to have outputs defined as ephemeral`) {
			t.Errorf("unexpected stderr: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
			t.Logf("sanitized serr: %s", sanitized)
		}
	})

	with("ephemeral_output_in_resource.tf", func() {
		tofuInit()
		sout, serr, err := tf.Run("apply")
		if err == nil || !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("unexpected err: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
		sanitized := SanitizeStderr(serr)
		if !strings.Contains(sanitized, `Ephemeral value used in non-ephemeral context    with simple_resource.test_res`) {
			t.Errorf("unexpected stderr: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
			t.Logf("sanitized serr: %s", sanitized)
		}
	})

	with("ephemeral_output_in_data_source.tf", func() {
		tofuInit()
		sout, serr, err := tf.Run("apply")
		if err == nil || !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("unexpected err: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
		sanitized := SanitizeStderr(serr)
		if !strings.Contains(sanitized, `Ephemeral value used in non-ephemeral context    with data.simple_resource.test_data1`) {
			t.Errorf("unexpected stderr: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
			t.Logf("sanitized serr: %s", sanitized)
		}
	})

	with("regular_output_given_ephemeral_value.tf", func() {
		tofuInit()
		sout, serr, err := tf.Run("apply")
		if err == nil || !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("unexpected err: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
		sanitized := SanitizeStderr(serr)
		filename := filepath.FromSlash("__mod-with-regular-output-got-ephemeral-value/main.tf")
		if !strings.Contains(sanitized, `Output does not allow ephemeral value    on `+filename) ||
			!strings.Contains(sanitized, `The value that was generated for the output is ephemeral, but it is not configured to allow one`) {
			t.Errorf("unexpected stderr: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
			t.Logf("sanitized serr: %s", sanitized)
		}
	})

	with("ephemeral_output_with_precondition.tf", func() {
		tofuInit()
		sout, serr, err := tf.Run("apply", "-var", "in=notdefaultvalue")
		if err == nil || !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("unexpected err: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
		}
		sanitizedSerr := SanitizeStderr(serr)
		sanitizedSout := SanitizeStderr(sout)
		filename := filepath.FromSlash("__mod-ephemeral-output-with-precondition/main.tf")
		if !strings.Contains(sanitizedSerr, `Module output value precondition failed    on `+filename) ||
			!strings.Contains(sanitizedSerr, `This check failed, but has an invalid error message as described in the other accompanying messages`) ||
			strings.Contains(sanitizedSerr, `"notdefaultvalue" -> "default value"`) {
			t.Errorf("unexpected stderr: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
			t.Logf("sanitized serr: %s", sanitizedSerr)
		}
		if !strings.Contains(sanitizedSout, `Warning: Error message refers to ephemeral values    on `+filename) ||
			!strings.Contains(sanitizedSout, `The error expression used to explain this condition refers to ephemeral values, so OpenTofu will not display the resulting message.  You can correct this by removing references to ephemeral values`) {
			t.Errorf("unexpected stdout: %s;\nstderr:\n%s\nstdout:\n%s", err, serr, sout)
			t.Logf("sanitized sout: %s", sanitizedSerr)
		}
	})
}
