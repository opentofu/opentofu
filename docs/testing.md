# Testing in the OpenTofu Codebase

> [!NOTE]  
> This document is about testing of the OpenTofu codebase itself for those contributing to OpenTofu, and _not_ about OpenTofu's features for testing end-user modules. Refer to [the `tofu test` subcommand](https://opentofu.org/docs/cli/commands/test/) to learn about how to test OpenTofu modules that you've written.

The OpenTofu codebase uses a number of different automated testing strategies in an attempt to avoid accidental regressions when implementing new features, and as part of the documentation for how each feature is intended to behave.

This document summarizes the different forms of test used in this codebase and provides some guidance on how to run the existing tests and add new tests of each type.

* [Unit Tests](#unit-tests)
* [Context Tests](#context-tests) (for `./internal/tofu` only)
* [Command Tests](#command-tests) (for `./internal/command` only)
* [Acceptance Tests](#acceptance-tests) (for testing against third-party network services)
* [End-to-end Tests](#end-to-end-tests) (for testing an already-built `tofu` executable)
* [Equivalence Tests](#equivalence-tests) (for detecting when certain important behaviors have changed)

# Unit Tests

The most pervasive kind of test, mandatory for almost all packages, are focused [unit tests](https://en.m.wikipedia.org/wiki/Unit_testing) directly exercising the exported API of a particular internal component.

You can run the unit tests using the `go test` command. For example, to run the tests for the entire codebase at once:

```shell
go test ./...
```

If you are working on functionality in a specific package then you might choose to focus on running only the tests for that package at first. Specify one or more packages to run on the command line, using the `...` suffix to represent "everything under this prefix":

```shell
go test ./internal/configs/... ./internal/lang/...
```

Some of the other testing types also run under `go test`, but most non-unit tests are skipped unless an environment variable is set to opt in, which we'll discuss in later sections.

OpenTofu unit tests follow the usual conventions for unit testing in Go. For more information, refer to the Go tutorial [Add a test](https://go.dev/doc/tutorial/add-a-test).

The scope of individual unit tests in OpenTofu varies depending on the context. Sometimes our test cover specific single functions, while other tests treat an entire package as the "unit", or somewhere in between. The main priority is in covering the intended behavior of a particular feature while exposing the test to as little ancillary behavior as possible so that a failure of a unit test is likely to make it clear exactly which component has malfunctioned and how it has malfunctioned.

# Context Tests

The main execution engine for OpenTofu, in the package `./internal/tofu`, involves a large number of highly-interconnected features that are often not very amenable to traditional unit testing, because most features are reachable only in conjunction with other features.

For example, testing the `ignore_changes` feature for resources in isolation as a unit test isn't really feasible because its behavior is just one small part of the overall resource instance change lifecycle. Any useful test for that feature tends to need to exercise a variety of other fundemental behaviors such as planning and applying changes across multiple rounds.

As a pragmatic compromise then, these language runtime behaviors tend to be tested "deeply" through the `tofu` package's main public entry point, [`tofu.Context`](https://pkg.go.dev/github.com/opentofu/opentofu@v1.8.2/internal/tofu#Context), by providing it with a suitable set of configuration files and calling the normal operations `Context.Plan`, `Context.Apply`, and so on. The tests then inspect the resulting plan and state artifacts to determine whether the features behaved as intended.

The `tofu` package refers to these as "context tests" due to the level of abstraction they test through, but these are essentially just [integration tests](https://en.m.wikipedia.org/wiki/Integration_testing) that happen to follow a particular set of conventions. These tests live in the `internal/tofu` directory in files whose names follow the pattern `context_*_test.go`. Due to the large number of these tests that has accumulated over time, some of the test files are arbitrarily split into multiple parts, such as `context_apply_test.go` and `context_apply2_test.go`. The distinction between these has no specific meaning, but entirely new tests are typically added to the file with the highest number unless a new test is thematically related to tests already in one of the other files.

Although the context tests have broader scope than unit tests, they don't have any network dependencies outside of this codebase and so they run as a default part of the full test suite, covered by `go test ./...`. However, since the functional scope of these tests is relatively large it's often helpful during development to initially run only a single test or small subset of tests at the same time. For example:

```shell
go test ./internal/tofu -run '^TestContext2Apply_destroyWithDataSourceExpansion$'
```

The `-run` option takes a regular expression pattern to match against the test function names. Thematically-related context tests tend to share a common prefix to make them easier to run together using this option.

**TODO:** Context tests probably deserve their own separate documentation file to capture the common patterns used in them, since they tend to all follow the same overall structure but it'll probably take a lot of words to describe that structure in full detail. In particular, the strategy for mocking the provider API without race conditions can be a little subtle and deserves some more detailed documentation coverage.

# Command Tests

The `./internal/command` package contains a mixture of both unit tests and integration-style tests, and "Command Tests" refers to the latter. Similar to the Context Tests described in the previous section, these tests intentionally go deep and execute important cross-cutting functionality that also includes behavior from other packages such as the configuration loader and the main language runtime.

There is currently no formal distinction between "command tests" and unit tests in the `./internal/command` package. You can run them in the same way as you'd run unit tests. You can typically recognize a "command test" by the use of the `metaOverridesForProvider` helper to provide mocked provider implementations that the language runtime will ultimately interact with during the test.

Command tests typically exercise other internal packages such as configuration loading and the language runtime, but _don't_ 

# Acceptance Tests

Some of the functionality in OpenTofu depends on external services reached over the network. For example, module and provider installation typically relies on registries or mirrors accessed over the network, and the most commonly-used state storage backends integrate with specific proprietary APIs offered by external vendors.

Since the behavior of these features depends strongly on the specific behavior of external services that can change independently of OpenTofu itself, the main testing vehicle for these is _acceptance tests_, which exercise relevant OpenTofu functionality in conjunction with the real external network service, rather than using test doubles such as mocks.

These tests typically depend either on running some specific software on your local computer or on having configured valid credentials for a remote SaaS API. Therefore these tests are skipped by default and opted-in by environment variables. The most common convention is to set the environment variable `TF_ACC=1` when running the tests, and to run only a specific package's tests or only a single test at a time during active development to decrease the setup overhead and round-trip time.

The details vary slightly depending on the specific needs of each package, but the overall approach is similar so we'll use the `pg` (Postgres) state storage backend from `./internal/backend/remote-state/pg` as a practical example:

1. Set up a Postgres server either on your local machine or on some remote system your development system can connect to. (The details of this are out of scope for this document, since they are specific to Postgres.)
2. Set the environment variable `DATABASE_URL` to contain a valid connection string for the Postgres server. This particular environment variable name is specific to the `pg` backend; other packages with acceptance tests each have their own environment variables for such settings.
3. Run the tests for the `pg` backend setting the environment variable `TF_ACC=1` to opt-in to running the acceptance tests:

    ```shell
    TF_ACC=1 go test ./internal/backend/remote-state/pg
    ```

Due to the reliance on external functionality that can change at any time outside of OpenTofu, our acceptance tests can unfortunately begin failing even when the corresponding functionality in OpenTofu hasn't changed. Therefore when you begin some new work on a package with acceptance tests it's often best to run the full set of acceptance tests for that package _before you make any changes_, and then deal with any failures you find before you begin other work.

Some of the acceptance tests have helpful wrappers in the project's `Makefile` that reduce or eliminate manual setup steps. To learn more, run:

```shell
make list-integration-tests
```

# End-to-end Tests

All of the test categories described above are executed using `go test` and so execute as part of a somewhat-contrived test program built locally on your system. That approach works fine for most functional testing, but cannot catch specific quirks like incorrectly-built final `tofu` executables.

The end-to-end tests are a special additional test suite that work by directly executing a real `tofu` executable, as an end-user might run it, and then checking that its results are as expected. This is essentially a more extreme version of integration test, intended to ensure that our final executable builds are constructed correctly.

The end-to-end tests are designed to be executable both using the normal `go test` flow (for ease of use during development) _and_ through a separate test runner that can run as part of the release process to verify our official builds.

If you use `go test ./...` to run all of the tests then you are already running the end-to-end tests. You can run them in isolation through `go test` as follows:

```shell
go test ./internal/command/e2etest/...
```

When run through `go test` the test program quietly uses `go build` to compile a temporary `tofu` executable based on the current source code in your work tree, and then each of the tests runs that executable, so you can largely treat the end-to-end tests in a similar way to other tests and trust that they will run as part of the overall test suite in your development environment.

Some of the end-to-end tests depend on external network services and so follow the same `TF_ACC=1` environment variable convention from Acceptance Tests to opt-in to those tests. However, unlike the acceptance tests the end-to-end tests depend only on OpenTofu-provided services that don't require authentication and so they don't require any additional configuration beyond opting in. Therefore we can and do opt in to these network-service-dependent tests in the verification step of our build process.

End-to-end tests are a mixed blessing: their broad scope allows them to catch certain kinds of problem that no other test category can catch, but it also makes them highly sensitive to false-negative failures as we make intentional changes to the rest of the product. Therefore end-to-end tests typically focus primarily on exercising [the primary workflow commands protected by the v1.x Compatibility Promises](https://opentofu.org/docs/language/v1-compatibility-promises/#protected-workflow-commands), while using more focus tests for other features.

The process for running the end-to-end tests against an official executable is a little more involved and unusual, and is not typically needed for everyday development, but you can find more information about it in [`internal/command/e2etest/make-archive.sh`](https://github.com/opentofu/opentofu/blob/main/internal/command/e2etest/make-archive.sh), and in the `e2e-test-build`, `e2e-test` and `e2e-test-exec` jobs in [the `build` GitHub Actions workflow](https://github.com/opentofu/opentofu/blob/main/.github/workflows/build.yml).

# Equivalence Tests

The "equivalence tests" are conceptually similar to the end-to-end tests but have an important difference: they are [characterization tests](https://en.m.wikipedia.org/wiki/Characterization_test) that involve comparing the entire output from a command to a previously-captured "known good" example, and so they represent how the software _actually_ behaved (in whatever version was most recently used to update the examples) rather than how it was _intended_ to behave.

The main consequence of this is that intentional user-visible changes to OpenTofu are likely to and are _expected_ to "break" the Equivalence Tests. When that happens the author must carefully review the differences and make sure they are all expected consequences of the changes they've made. If the author concludes that all of the detected changes are intentional then they should capture a fresh set of example outputs that represent the updated "known-good" behavior to use as the basis for future tests.

For more information on equivalence tests and how to update them, refer to [the Equivalence Tests README](../testing/equivalence-tests/README.md).
