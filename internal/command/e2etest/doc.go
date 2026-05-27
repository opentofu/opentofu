// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package e2etest contains a set of tests that run against a real OpenTofu
// binary, compiled on the fly at the start of the test run.
//
// These tests help ensure that key end-to-end OpenTofu use-cases are working
// for a real binary, whereas other tests always have at least _some_ amount
// of test stubbing.
//
// The goal of this package is not to duplicate the functional testing done
// in other packages but rather to fully exercise a few important workflows
// in a realistic way.
//
// You can run these tests using "go test" as usual:
//
//	go test -v github.com/opentofu/opentofu/internal/command/e2etest
//
// This will compile on the fly a OpenTofu binary and run the tests against
// it.
//
// The TF_ACC environment variable must be set for the tests to reach out
// to external network services. Since these are end-to-end tests, only a
// few very basic tests can execute without this environment variable set.
//
// # Scope for end-to-end tests
//
// End-to-end tests are useful in that they test behaviors of the whole system
// all wired together, so they can expose bugs in the interactions between
// components that our unit tests would not catch.
//
// However, they also tend to be brittle because of how much behavior they each
// cover, and because at this granularity we can't reliably distinguish behavior
// protected by compatibility promises from behavior that's just a current
// implementation detail or human-oriented UI choice. When a test in this
// package unexpectedly fails, it tends to be time-consuming and challenging to
// identify the root cause, and that's particularly frustrating if the behavior
// in question was not something that was covered by compatibility promises
// anyway.
//
// Because of that tension, we don't aspire for anything close to 100% coverage
// of OpenTofu's features by tests in this package. Instead, out of pragmatism
// we typically limit the scope of this package to the following situations:
//
//   - A fuzzily-defined subset of features that we think of as the
//     "primary workflow", verifying that the usual init/validate/plan/apply
//     sequence is generally functional without exhaustively testing all possible
//     language features. This includes both the primary workflow as often used
//     manually at a command line, where "tofu apply" is used to both plan and
//     apply in a single command, and the primary workflow used by automation
//     systems (e.g. TACOS) where plan and apply happen as separate commands
//     connected using a saved plan file.
//   - Specific functionality whose correct behavior relies on correctly wiring
//     together architecturally-separate components. For example, we tests some
//     aspects of provider installation with end-to-end tests because the
//     configurable installation methods rely on interactions between the CLI
//     configuration system, the "tofu init" command, our shared mechanism for
//     authenticating to OpenTofu-native services, and of course the provider
//     installer itself.
//
// Overall we prefer to write unit tests or more focused integration tests
// for anything that isn't clearly in one of the above categories, unless we
// have no other viable option.
//
// # How to write end-to-end tests
//
// Our end-to-end tests are structured like normal Go tests as much as possible,
// but there are some special touches that are unique to this package:
//
//   - Any test that could potentially cause the OpenTofu CLI process being run
//     to access network services aside from those offered by the test itself
//     on the loopback interface MUST call "skipIfCannotAccessNetwork" early
//     in its execution to ensure that it will run only when the "TF_ACC"
//     environment variable is set. We typically include a comment just above
//     the call to that function explaining what external network access the
//     test is expected to make, so that those who want to run the test know
//     what to expect.
//   - Even tests that access external network services must only access
//     services directly controlled by the OpenTofu project, such as OpenTofu's
//     public registry. We don't write end-to-end tests that access third-party
//     services such as GitHub where we have no influence over the availability
//     of the service and where our automated runs of these tests as part of
//     pull request checks would cause nuisance requests to the target.
//   - We use the helpers in [e2e] to run the OpenTofu CLI executable that
//     gets built automatically in TestMain. Tests therefore typically call
//     [e2e.NewBinary] at some point during their setup code and then use the
//     resulting object to run OpenTofu inside a temporary working directory.
//     The package-level variable "tofuBin" contains the path to the temporary
//     "tofu" executable to use for testing.
//   - When testing human-oriented output (as opposed to than machine-oriented
//     output), try to constrain the expected output as little as possible while
//     minimizing the risk of false-positives, since human-oriented output is
//     explicitly not covered by compatibility promises and so we want to match
//     output only as a proxy for whether the expected internal behavior
//     occurred and would prefer not to have to constantly update theses tests
//     every time we make small UI improvements.
//   - If you are running commands with virtual terminal color codes enabled,
//     consider using the "stripAnsi" helper function to filter out escape
//     sequences before comparing the output.
//   - If possible, write tests to be safe to run concurrently with other tests
//     and call "t.Parallel()" at the start of the function to indicate that.
//
// The following is some example test setup boilerplate:
//
//	    func TestSomething(t *testing.T) {
//	        t.Parallel()
//
//	        // This test accesses registry.opentofu.org to to download the
//	        // hashicorp/null provider.
//	        skipIfCannotAccessNetwork(t)
//
//		    fixturePath := filepath.Join("testdata", "example-fixture")
//		    tf := e2e.NewBinary(t, tofuBin, fixturePath)
//
//		    stdout, stderr, err := tf.Run("init")
//		    if err != nil {
//	            t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
//	        }
//
//	        // ...and so on, with relatively normal-looking Go test code
//	    }
package e2etest

import (
	"github.com/opentofu/opentofu/internal/e2e"
)

func _() {
	// This is here just to get the import of package e2e above so that our
	// doc comment can include links to it.
	_ = e2e.NewBinary
}
