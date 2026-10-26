// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestBackendMigrate_promptMultiStatePattern(t *testing.T) {
	// Setup the meta

	cases := map[string]struct {
		renamePrompt  string
		patternPrompt string
		expectedErr   string
	}{
		"valid pattern": {
			renamePrompt:  "1",
			patternPrompt: "hello-*",
			expectedErr:   "",
		},
		"invalid pattern, only one asterisk allowed": {
			renamePrompt:  "1",
			patternPrompt: "hello-*-world-*",
			expectedErr:   "The pattern '*' cannot be used more than once.",
		},
		"invalid pattern, missing asterisk": {
			renamePrompt:  "1",
			patternPrompt: "hello-world",
			expectedErr:   "The pattern must have an '*'",
		},
		"invalid rename": {
			renamePrompt: "3",
			expectedErr:  "Please select 1 or 2 as part of this option.",
		},
		"no rename": {
			renamePrompt: "2",
		},
	}
	for name, tc := range cases {
		t.Log("Test: ", name)
		m := testMetaBackend(t)
		input := map[string]string{}
		cleanup := testInputMap(t, input)
		if tc.renamePrompt != "" {
			input["backend-migrate-multistate-to-tfc"] = tc.renamePrompt
		}
		if tc.patternPrompt != "" {
			input["backend-migrate-multistate-to-tfc-pattern"] = tc.patternPrompt
		}

		sourceType := "cloud"
		_, err := m.promptMultiStateMigrationPattern(context.Background(), sourceType)
		if tc.expectedErr == "" && err != nil {
			t.Fatalf("expected error to be nil, but was %s", err.Error())
		}
		if tc.expectedErr != "" && tc.expectedErr != err.Error() {
			t.Fatalf("expected error to eq %s but got %s", tc.expectedErr, err.Error())
		}

		cleanup()
	}
}

func TestBackendMigrate_cancelDuringInputReading(t *testing.T) {
	m := testMetaBackend(t)

	// force it to run like a "real" tofu command
	test = false
	defer func() { test = true }()

	// Create a cancelled context to see if it is passed (next I will ask the input to be a pipe so it works)
	ctx, cancel := context.WithCancel(context.Background())

	// make suer that the actual call will use the cancellable context
	m.CallerContext = ctx

	// create a pipe that is idle
	rx, tx := io.Pipe()

	// cancel the context right before you start reading
	var once sync.Once
	rWrapped := notifyReader{
		cancel: cancel,
		once:   &once,
		Reader: rx,
	}

	// and force the code to use it so it hangs on input for address
	defaultInputReader = &rWrapped

	// reset all things I set when done to not hog resources
	defer func() {
		defaultInputReader = os.Stdin
		rx.Close()
		tx.Close()
	}()

	sourceType := "cloud"
	_, err := m.promptMultiStateMigrationPattern(ctx, sourceType)

	if err == nil {
		t.Fatal("expected an err to occur")
	}

	if !strings.Contains(err.Error(), "interrupted: context canceled") {
		t.Fatalf("error was not the one I expected to have, %s", err)
	}

}
