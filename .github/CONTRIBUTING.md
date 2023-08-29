# Contributing to OpenTF

This repository contains OpenTF core, which includes the command line interface and the main graph engine

This document provides guidance on OpenTF contribution recommended practices. It covers what we're looking for in order to help set some expectations and help you get the most out of participation in this project.

To record a bug report, enhancement proposal, or give any other product feedback, please [open a GitHub issue](https://github.com/placeholderplaceholderplaceholder/opentf/issues/new/choose) using the most appropriate issue template

The contribution guideline will change in the near future, as the management of the open-source project is stabilized

---

<!-- MarkdownTOC autolink="true" -->

- [Proposing a Change](#proposing-a-change)
- [PR Checks](#pr-checks)
- [OpenTF CLI/Core Development Environment](#opentf-clicore-development-environment)
- [Acceptance Tests: Testing interactions with external services](#acceptance-tests-testing-interactions-with-external-services)
- [Generated Code](#generated-code)

<!-- /MarkdownTOC -->

## Proposing a Change

In order to be respectful of the time of community contributors, we aim to discuss potential changes in GitHub issues prior to implementation. That will allow us to give design feedback up front and set expectations about the scope of the change, and, for larger changes, how best to approach the work such that the OpenTF team can review it and merge it along with other concurrent work.

If the bug you wish to fix or enhancement you wish to implement isn't already covered by a GitHub issue, please do start a discussion (either in [a new GitHub issue](https://github.com/placeholderplaceholderplaceholder/opentf/issues/new/choose) or an existing one, as appropriate) before you invest significant development time.

### PR Checks

Test checks run when a PR is opened. Tests include unit tests and acceptance tests, and all tests must pass before a PR can be merged

----

## OpenTF CLI/Core Development Environment

This repository contains the source code for OpenTF CLI, which is the main component of OpenTF that contains the core OpenTF engine.

---

If you wish to work on the OpenTF CLI source code, you'll first need to install the [Go](https://golang.org/) compiler and the version control system [Git](https://git-scm.com/).

At this time the OpenTF development environment is targeting only Linux and Mac OS X systems. While OpenTF itself is compatible with Windows, unfortunately the unit test suite currently contains Unix-specific assumptions around maximum path lengths, path separators, etc.

Refer to the file [`.go-version`](https://github.com/placeholderplaceholderplaceholder/opentf/blob/main/.go-version) to see which version of Go OpenTF is currently built with. Other versions will often work, but if you run into any build or testing problems please try with the specific Go version indicated. You can optionally simplify the installation of multiple specific versions of Go on your system by installing [`goenv`](https://github.com/syndbg/goenv), which reads `.go-version` and automatically selects the correct Go version.

Use Git to clone this repository into a location of your choice. OpenTF is using [Go Modules](https://blog.golang.org/using-go-modules), and so you should *not* clone it inside your `GOPATH`.

Switch into the root directory of the cloned repository and build OpenTF using the Go toolchain in the standard way:

```
cd opentf
go install .
```

The first time you run the `go install` command, the Go toolchain will download any library dependencies that you don't already have in your Go modules cache. Subsequent builds will be faster because these dependencies will already be available on your local disk.

Once the compilation process succeeds, you can find a `opentf` executable in the Go executable directory. If you haven't overridden it with the `GOBIN` environment variable, the executable directory is the `bin` directory inside the directory returned by the following command:

```
go env GOPATH
```

If you are planning to make changes to the OpenTF source code, you should run the unit test suite before you start to make sure everything is initially passing:

```
go test ./...
```

As you make your changes, you can re-run the above command to ensure that the tests are *still* passing. If you are working only on a specific Go package, you can speed up your testing cycle by testing only that single package, or packages under a particular package prefix:

```
go test ./internal/command/...
go test ./internal/addrs
```

## Acceptance Tests: Testing interactions with external services

OpenTF's unit test suite is self-contained, using mocks and local files to help ensure that it can run offline and is unlikely to be broken by changes to outside systems.

However, several OpenTF components interact with external services.

There are some optional tests in the OpenTF CLI codebase that *do* interact with external services, which we collectively refer to as "acceptance tests". You can enable these by setting the environment variable `TF_ACC=1` when running the tests. We recommend focusing only on the specific package you are working on when enabling acceptance tests, both because it can help the test run to complete faster and because you are less likely to encounter failures due to drift in systems unrelated to your current goal:

```
TF_ACC=1 go test ./internal/initwd
```

Because the acceptance tests depend on services outside of the OpenTF codebase, and because the acceptance tests are usually used only when making changes to the systems they cover, it is common and expected that drift in those external systems will cause test failures. Because of this, prior to working on a system covered by acceptance tests it's important to run the existing tests for that system in an *unchanged* work tree first and respond to any test failures that preexist, to avoid misinterpreting such failures as bugs in your new changes.

## Generated Code

Some files in the OpenTF CLI codebase are generated. In most cases, we update these using `go generate`, which is the standard way to encapsulate code generation steps in a Go codebase.

```
go generate ./...
```

Use `git diff` afterwards to inspect the changes and ensure that they are what you expected.

OpenTF includes generated Go stub code for the OpenTF provider plugin protocol, which is defined using Protocol Buffers. Because the Protocol Buffers tools are not written in Go and thus cannot be automatically installed using `go get`, we follow a different process for generating these, which requires that you've already installed a suitable version of `protoc`:

```
make protobuf
```
