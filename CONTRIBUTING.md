# Contributing to OpenTofu

This repository contains OpenTofu Core, which includes the command line interface, the main graph engine, and the documentation for them.

This document provides guidance on OpenTofu contribution recommended practices. It covers how to submit issues, how to get involved in the discussion, how to work on the code, and how to contribute code changes.

The easiest way to contribute is by [opening an issue](https://github.com/opentofu/opentofu/issues/new/choose)! Bug reports, broken compatibility reports, feature requests, old issue reposts, and well-prepared RFCs are all very welcome.

All major changes to OpenTofu Core go through the public RFC process, including those proposed by the core team. Thus, if you'd like to propose such a change, please prepare an RFC, so that the community can discuss the change and everybody has a chance to voice their opinion. You're also welcome to voice your own opinion on existing RFCs! You can find them by [going to the issues view and filtering by the rfc label](https://github.com/opentofu/opentofu/issues?q=is%3Aopen+is%3Aissue+label%3Arfc).

Generally, we appreciate external contributions very much and would love to work with you on them. **However, please make sure to read the [Contributing a Code Change](#contributing-a-code-change) section prior to making a contribution.**

**Important Note: Since we're still in the cleanup phase of making this repository ready for the first alpha release, we encourage you to wait with any code contributions until this first alpha release is out, to avoid conflicts.**

---

<!-- MarkdownTOC autolink="true" -->

- [Contributing a Code Change](#contributing-a-code-change)
- [Working on the Code](#working-on-the-code)
- [Acceptance Tests: Testing interactions with external services](#acceptance-tests-testing-interactions-with-external-services)
- [Generated Code](#generated-code)

<!-- /MarkdownTOC -->

## Contributing a Code Change

In order to contribute a code change, you should fork the repository, make your changes, and then submit a pull request. Crucially, all code changes should be preceded by an issue that you've been assigned to. If an issue for the change you'd like to introduce already exists, please communicate in the issue that you'd like to take ownership of it. If an issue doesn't yet exist, please create one expressing your interest in working on it and discuss it first, prior to working on the code. Code changes without a related issue will generally be rejected.

Only issues with the `accepted` label have been officially accepted for implementation, so please avoid working on issues without that label.

In order for a code change to be accepted, you'll also have to accept the Developer Certificate of Origin (DCO). It's very lightweight, and you can find it [here](https://developercertificate.org). Accepting is accomplished by signing off on your commits, you can do this by adding a `Signed-off-by` line to your commit message, like here:
```
This is my commit message

Signed-off-by: Random Developer <random@developer.example.org>
```
Git has a built-in flag to append this line automatically:
```
~> git commit -s -m 'This is my commit message'
```

You can find more details about the DCO checker in the [DCO app repo](https://github.com/dcoapp/app).

Additionally, please update [the changelog](CHANGELOG.md) if you're making any user-facing changes.

## Working on the Code

If you wish to work on the OpenTofu CLI source code, you'll first need to install the [Go](https://golang.org/) compiler and the version control system [Git](https://git-scm.com/).

At this time the OpenTofu development environment is targeting only Linux and Mac OS X systems. While OpenTofu itself is compatible with Windows, unfortunately the unit test suite currently contains Unix-specific assumptions around maximum path lengths, path separators, etc.

Refer to the file [`.go-version`](.go-version) to see which version of Go OpenTofu is currently built with. Other versions will often work, but if you run into any build or testing problems please try with the specific Go version indicated. You can optionally simplify the installation of multiple specific versions of Go on your system by installing [`goenv`](https://github.com/syndbg/goenv), which reads `.go-version` and automatically selects the correct Go version.

Use Git to clone this repository into a location of your choice. OpenTofu is using [Go Modules](https://blog.golang.org/using-go-modules), and so you should *not* clone it inside your `GOPATH`.

Switch into the root directory of the cloned repository and build OpenTofu using the Go toolchain in the standard way:

```
cd opentofu
go install ./cmd/tofu
```

The first time you run the `go install` command, the Go toolchain will download any library dependencies that you don't already have in your Go modules cache. Subsequent builds will be faster because these dependencies will already be available on your local disk.

Once the compilation process succeeds, you can find a `tofu` executable in the Go executable directory. If you haven't overridden it with the `GOBIN` environment variable, the executable directory is the `bin` directory inside the directory returned by the following command:

```
go env GOPATH
```

If you are planning to make changes to the OpenTofu source code, you should run the unit test suite before you start to make sure everything is initially passing:

```
go test ./...
```

As you make your changes, you can re-run the above command to ensure that the tests are *still* passing. If you are working only on a specific Go package, you can speed up your testing cycle by testing only that single package, or packages under a particular package prefix:

```
go test ./internal/command/...
go test ./internal/addrs
```

## Adding or updating dependencies

If you need to add or update dependencies, you'll have to make sure they use only approved and compatible licenses. The list of these licenses is defined in [`.licensei.toml`](.licensei.toml).

To help verifying this in local development environment and in continuous integration, we use the [licensei](https://github.com/goph/licensei) open source tool.

After modifying `go.mod` or `go.sum` files, you can run it manually with:

```
export GITHUB_TOKEN=changeme
make license-check
```

Note: you need to define the `GITHUB_TOKEN` environment variable to a valid GitHub personal access token, or you will hit rate limiting from the GitHub API which `licensei` uses to discover the licenses of dependencies.

## Acceptance Tests: Testing interactions with external services

OpenTofu's unit test suite is self-contained, using mocks and local files to help ensure that it can run offline and is unlikely to be broken by changes to outside systems.

However, several OpenTofu components interact with external services.

There are some optional tests in the OpenTofu CLI codebase that *do* interact with external services, which we collectively refer to as "acceptance tests". You can enable these by setting the environment variable `TF_ACC=1` when running the tests. We recommend focusing only on the specific package you are working on when enabling acceptance tests, both because it can help the test run to complete faster and because you are less likely to encounter failures due to drift in systems unrelated to your current goal:

```
TF_ACC=1 go test ./internal/initwd
```

Because the acceptance tests depend on services outside of the OpenTofu codebase, and because the acceptance tests are usually used only when making changes to the systems they cover, it is common and expected that drift in those external systems will cause test failures. Because of this, prior to working on a system covered by acceptance tests it's important to run the existing tests for that system in an *unchanged* work tree first and respond to any test failures that preexist, to avoid misinterpreting such failures as bugs in your new changes.

## Generated Code

Some files in the OpenTofu CLI codebase are generated. In most cases, we update these using `go generate`, which is the standard way to encapsulate code generation steps in a Go codebase.

```
go generate ./...
```

Use `git diff` afterwards to inspect the changes and ensure that they are what you expected.

OpenTofu includes generated Go stub code for the OpenTofu provider plugin protocol, which is defined using Protocol Buffers. Because the Protocol Buffers tools are not written in Go and thus cannot be automatically installed using `go get`, we follow a different process for generating these, which requires that you've already installed a suitable version of `protoc`:

```
make protobuf
```
