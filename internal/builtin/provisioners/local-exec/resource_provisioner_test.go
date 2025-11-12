// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package localexec

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/zclconf/go-cty/cty"
)

func TestResourceProvider_Apply(t *testing.T) {
	defer os.Remove("test_out")
	output := cli.NewMockUi()
	p := New()
	schema := p.GetSchema().Provisioner
	c, err := schema.CoerceValue(cty.ObjectVal(map[string]cty.Value{
		"command": cty.StringVal("echo foo > test_out"),
	}))
	if err != nil {
		t.Fatal(err)
	}

	resp := p.ProvisionResource(provisioners.ProvisionResourceRequest{
		Config:   c,
		UIOutput: output,
	})

	if resp.Diagnostics.HasErrors() {
		t.Fatalf("err: %v", resp.Diagnostics.Err())
	}

	// Check the file
	raw, err := os.ReadFile("test_out")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	actual := strings.TrimSpace(string(raw))
	expected := "foo"
	if actual != expected {
		t.Fatalf("bad: %#v", actual)
	}
}

func TestResourceProvider_stop(t *testing.T) {
	output := cli.NewMockUi()
	p := New()
	schema := p.GetSchema().Provisioner

	command := "sleep 30; sleep 30"
	if runtime.GOOS == "windows" {
		// On Windows the local-exec provisioner uses cmd.exe by default,
		// and that uses "&" as a command separator instead of ";".
		command = "sleep 30 & sleep 30"
	}

	c, err := schema.CoerceValue(cty.ObjectVal(map[string]cty.Value{
		// bash/zsh/ksh will exec a single command in the same process. This
		// makes certain there's a subprocess in the shell.
		"command": cty.StringVal(command),
	}))
	if err != nil {
		t.Fatal(err)
	}

	doneCh := make(chan struct{})
	var provisionerResp atomic.Pointer[provisioners.ProvisionResourceResponse]
	go func() {
		defer close(doneCh)
		resp := p.ProvisionResource(provisioners.ProvisionResourceRequest{
			Config:   c,
			UIOutput: output,
		})
		provisionerResp.Store(&resp)
	}()

	mustExceed := (250 * time.Millisecond)
	select {
	case <-doneCh:
		// If doneCh is closed here then provisionerResp will have been
		// set to a non-nil pointer and so we'll catch it in the
		// if statement immediately below.
	case <-time.After(mustExceed):
		t.Logf("correctly took longer than %s", mustExceed)
	}
	if resp := provisionerResp.Load(); resp != nil {
		// This catches a potential misleading outcome where the provisioner
		// exits early due to an error but does so slow enough that it would
		// pass the minimum time check above.
		if resp.Diagnostics.HasErrors() {
			t.Fatalf("provisioner failed: %s", resp.Diagnostics.Err())
		}
		t.Fatalf("provisioner responded with success before we asked it to stop")
	}

	// Stop it
	stopTime := time.Now()
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}

	maxTempl := "expected to finish under %s, finished in %s"
	finishWithin := (2 * time.Second)
	select {
	case <-doneCh:
		t.Logf(maxTempl, finishWithin, time.Since(stopTime))
	case <-time.After(finishWithin):
		t.Fatalf(maxTempl, finishWithin, time.Since(stopTime))
	}

	// Our background goroutine _must_ eventually exit before we consider
	// the test to be done.
	for range doneCh {
	}
}

func TestResourceProvider_ApplyCustomInterpreter(t *testing.T) {
	output := cli.NewMockUi()
	p := New()

	schema := p.GetSchema().Provisioner

	c, err := schema.CoerceValue(cty.ObjectVal(map[string]cty.Value{
		"interpreter": cty.ListVal([]cty.Value{cty.StringVal("echo"), cty.StringVal("is")}),
		"command":     cty.StringVal("not really an interpreter"),
	}))
	if err != nil {
		t.Fatal(err)
	}

	resp := p.ProvisionResource(provisioners.ProvisionResourceRequest{
		Config:   c,
		UIOutput: output,
	})

	if resp.Diagnostics.HasErrors() {
		t.Fatal(resp.Diagnostics.Err())
	}

	got := strings.TrimSpace(output.OutputWriter.String())
	want := `Executing: ["echo" "is" "not really an interpreter"]
is not really an interpreter`
	if got != want {
		t.Errorf("wrong output\ngot:  %s\nwant: %s", got, want)
	}
}

func TestResourceProvider_ApplyCustomWorkingDirectory(t *testing.T) {
	testdir := "working_dir_test"
	if err := os.Mkdir(testdir, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testdir)

	output := cli.NewMockUi()
	p := New()
	schema := p.GetSchema().Provisioner

	command := "echo `pwd`"
	if runtime.GOOS == "windows" {
		command = "echo %cd%"
	}
	c, err := schema.CoerceValue(cty.ObjectVal(map[string]cty.Value{
		"working_dir": cty.StringVal(testdir),
		"command":     cty.StringVal(command),
	}))
	if err != nil {
		t.Fatal(err)
	}

	resp := p.ProvisionResource(provisioners.ProvisionResourceRequest{
		Config:   c,
		UIOutput: output,
	})

	if resp.Diagnostics.HasErrors() {
		t.Fatal(resp.Diagnostics.Err())
	}

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	got := strings.TrimSpace(output.OutputWriter.String())
	want := "Executing: [\"/bin/sh\" \"-c\" \"echo `pwd`\"]\n" + dir + "/" + testdir
	if runtime.GOOS == "windows" {
		want = "Executing: [\"cmd\" \"/C\" \"echo %cd%\"]\n" + dir + "\\" + testdir
	}
	if got != want {
		t.Errorf("wrong output\ngot:  %s\nwant: %s", got, want)
	}
}

func TestResourceProvider_ApplyCustomEnv(t *testing.T) {
	output := cli.NewMockUi()
	p := New()
	schema := p.GetSchema().Provisioner
	command := "echo $FOO $BAR $BAZ"
	if runtime.GOOS == "windows" {
		command = "echo %FOO% %BAR% %BAZ%"
	}
	c, err := schema.CoerceValue(cty.ObjectVal(map[string]cty.Value{
		"command": cty.StringVal(command),
		"environment": cty.MapVal(map[string]cty.Value{
			"FOO": cty.StringVal("BAR"),
			"BAR": cty.StringVal("1"),
			"BAZ": cty.StringVal("true"),
		}),
	}))
	if err != nil {
		t.Fatal(err)
	}

	resp := p.ProvisionResource(provisioners.ProvisionResourceRequest{
		Config:   c,
		UIOutput: output,
	})
	if resp.Diagnostics.HasErrors() {
		t.Fatal(resp.Diagnostics.Err())
	}

	got := strings.TrimSpace(output.OutputWriter.String())

	want := "Executing: [\"/bin/sh\" \"-c\" \"echo $FOO $BAR $BAZ\"]\nBAR 1 true"
	if runtime.GOOS == "windows" {
		want = "Executing: [\"cmd\" \"/C\" \"echo %FOO% %BAR% %BAZ%\"]\nBAR 1 true"
	}

	if got != want {
		t.Errorf("wrong output\ngot:  %s\nwant: %s", got, want)
	}
}

// Validate that Stop can Close can be called even when not provisioning.
func TestResourceProvisioner_StopClose(t *testing.T) {
	p := New()
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	p.Close()
}

func TestResourceProvisioner_nullsInOptionals(t *testing.T) {
	output := cli.NewMockUi()
	p := New()
	schema := p.GetSchema().Provisioner

	for i, cfg := range []cty.Value{
		cty.ObjectVal(map[string]cty.Value{
			"command": cty.StringVal("echo OK"),
			"environment": cty.MapVal(map[string]cty.Value{
				"FOO": cty.NullVal(cty.String),
			}),
		}),
		cty.ObjectVal(map[string]cty.Value{
			"command":     cty.StringVal("echo OK"),
			"environment": cty.NullVal(cty.Map(cty.String)),
		}),
		cty.ObjectVal(map[string]cty.Value{
			"command":     cty.StringVal("echo OK"),
			"interpreter": cty.ListVal([]cty.Value{cty.NullVal(cty.String)}),
		}),
		cty.ObjectVal(map[string]cty.Value{
			"command":     cty.StringVal("echo OK"),
			"interpreter": cty.NullVal(cty.List(cty.String)),
		}),
		cty.ObjectVal(map[string]cty.Value{
			"command":     cty.StringVal("echo OK"),
			"working_dir": cty.NullVal(cty.String),
		}),
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {

			cfg, err := schema.CoerceValue(cfg)
			if err != nil {
				t.Fatal(err)
			}

			// verifying there are no panics
			p.ProvisionResource(provisioners.ProvisionResourceRequest{
				Config:   cfg,
				UIOutput: output,
			})
		})
	}
}
