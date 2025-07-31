// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package communicator

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/zclconf/go-cty/cty"
)

func TestCommunicator_new(t *testing.T) {
	cfg := map[string]cty.Value{
		"type": cty.StringVal("telnet"),
		"host": cty.StringVal("127.0.0.1"),
	}

	if _, err := New(cty.ObjectVal(cfg)); err == nil {
		t.Fatalf("expected error with telnet")
	}

	cfg["type"] = cty.StringVal("ssh")
	if _, err := New(cty.ObjectVal(cfg)); err != nil {
		t.Fatalf("err: %v", err)
	}

	cfg["type"] = cty.StringVal("winrm")
	if _, err := New(cty.ObjectVal(cfg)); err != nil {
		t.Fatalf("err: %v", err)
	}
}
func TestRetryFunc(t *testing.T) {
	origMax := maxBackoffDelay
	maxBackoffDelay = time.Second
	origStart := initialBackoffDelay
	initialBackoffDelay = 10 * time.Millisecond

	defer func() {
		maxBackoffDelay = origMax
		initialBackoffDelay = origStart
	}()

	// succeed on the third try
	errs := []error{io.EOF, &net.OpError{Err: errors.New("ERROR")}, nil}
	count := 0

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := Retry(ctx, func() error {
		if count >= len(errs) {
			return errors.New("failed to stop after nil error")
		}

		err := errs[count]
		count++

		return err
	})

	if count != 3 {
		t.Fatal("retry func should have been called 3 times")
	}

	if err != nil {
		t.Fatal(err)
	}
}

func TestRetryFuncBackoff(t *testing.T) {
	origMax := maxBackoffDelay
	maxBackoffDelay = time.Second
	origStart := initialBackoffDelay
	initialBackoffDelay = 100 * time.Millisecond

	retryTestWg = &sync.WaitGroup{}
	retryTestWg.Add(1)

	defer func() {
		maxBackoffDelay = origMax
		initialBackoffDelay = origStart
		retryTestWg = nil
	}()

	count := 0

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := Retry(ctx, func() error {
		count++
		return io.EOF
	})
	if !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	cancel()
	retryTestWg.Wait()

	if count > 4 {
		t.Fatalf("retry func failed to backoff. called %d times", count)
	}
}
