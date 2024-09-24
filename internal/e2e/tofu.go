package e2e

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
)

// Tofu command builder
type Tofu struct {
	args     []string
	dir      string
	scenario *Scenario
	t        *testing.T
	env      map[string]string
}

func (t *Tofu) SetEnv(name string, value string) *Tofu {
	t.env[name] = value
	return t
}

func (t *Tofu) T() *testing.T {
	return t.t
}
func (t *Tofu) Scenario() *Scenario {
	return t.scenario
}

type syncWriter struct {
	io.Writer
	mu *sync.Mutex
}

func (sw syncWriter) Write(b []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.Writer.Write(b)
}

func (t *Tofu) Run() *Result {
	result := &Result{
		t:      t.t,
		stdall: new(bytes.Buffer),
		stdout: new(bytes.Buffer),
		stderr: new(bytes.Buffer),
	}

	binPath := "/home/cmesh/go/bin/tofu" // TODO
	cmd := exec.Command(binPath, t.args...)
	// TODO check if binary exists

	cmd.Dir = t.dir
	cmd.Env = os.Environ()
	for k, v := range t.env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	cmd.Stdin = nil // TODO
	var mu sync.Mutex
	// Synchronize writes to the stdall buffer as race conditions can occur between the stdout pipe writer and stderr pipewriter
	// There's a workaround in the stdlib that handles the case where the out and error are the *same* writer.  In this case, the
	// io.MultiWriter contains the stdall buffer, but there is no way for the builtin protection to be enabled.
	// Thus we sync calls to Write() from the same process.  This took wayyyy to long to figure out.
	cmd.Stdout = syncWriter{io.MultiWriter(result.stdall, result.stdout), &mu}
	cmd.Stderr = syncWriter{io.MultiWriter(result.stdall, result.stderr), &mu}

	err := cmd.Start()
	if err != nil {
		t.t.Fatal(err)
	}
	result.err = cmd.Wait()

	return result
}

type Result struct {
	t      *testing.T
	err    error
	stdall *bytes.Buffer
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func (r *Result) Success() *Result {
	if r.err != nil {
		r.t.Errorf("Expected success, got error %s: \n %s", r.err, string(r.stdall.Bytes()))
	}
	return r
}

func (r *Result) Failure() *Result {
	if r.err == nil {
		r.t.Error("Expected error, command ran without error")
	}
	return r
}

func (r *Result) Output() *Output {
	return &Output{t: r.t, raw: r.stdall.Bytes()}
}
func (r *Result) Stdout() *Output {
	return &Output{t: r.t, raw: r.stdout.Bytes()}
}
func (r *Result) Stderr() *Output {
	return &Output{t: r.t, raw: r.stderr.Bytes()}
}
