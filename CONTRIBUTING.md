# Contributing to OpenTofu

Welcome and thank you for wanting to contribute! 

## In a hurry? Here's the short version:

- Have a question? 💬 Post it in [GitHub Discussions](https://github.com/orgs/opentofu/discussions) or on the [OpenTofu Slack](https://opentofu.org/slack/)!
- Found a bug? [Report it here ➡️](https://github.com/opentofu/opentofu/issues/new?assignees=&labels=bug%2Cpending-decision&projects=&template=bug_report.yml)
- Have a feature idea? [Submit it here ➡️](https://github.com/opentofu/opentofu/issues/new?assignees=&labels=enhancement%2Cpending-decision&projects=&template=feature_request.yml)
- Want to provide detailed documentation on a feature idea? [Write an RFC here ➡️](https://github.com/opentofu/opentofu/issues/new?assignees=&labels=rfc%2Cpending-decision&projects=&template=rfc.yml)
- Want to provide a proof-of-concept for an issue? Please [submit a draft PR here ➡️](https://github.com/opentofu/opentofu/compare)
- Want to add a feature, fix a linter error, refactor something, or add CI tooling?
  1. Check if there is an [open issue with the `accepted` label](https://github.com/opentofu/opentofu/issues?q=is%3Aopen+is%3Aissue+label%3Aaccepted),
  2. Comment on the issue that you want to work on it,
  3. Wait for a maintainer to assign it to you,
  4. Then [submit your code here ➡️](https://github.com/opentofu/opentofu/compare)
- Want to fix a bug? [Submit a PR here ➡️](https://github.com/opentofu/opentofu/compare)

**⚠️ Important:** Please avoid working on features or refactor without [an `accepted` issue](https://github.com/opentofu/opentofu/issues?q=is%3Aopen+is%3Aissue+label%3Aaccepted). OpenTofu is a large and complex project and every change needs careful consideration. We cannot merge non-bug pull requests without first having a discussion about them, no matter how trivial the issue may seem.

We specifically do not merge PRs **without prior issues** that:

- Reformat code
- Rename things
- Move code around
- Fix linter warnings for tools not currently in the CI pipeline
- Add new CI tooling

## Long version

This repository contains OpenTofu Core, which includes the command line interface, the main graph engine, and the documentation for them.

This document provides guidance on OpenTofu contribution recommended practices. It covers how to submit issues, how to get involved in the discussion, how to work on the code, and how to contribute code changes.

The easiest way to contribute is by [opening an issue](https://github.com/opentofu/opentofu/issues/new/choose)! Bug reports, broken compatibility reports, feature requests, old issue reposts, and well-prepared RFCs are all very welcome.

All major changes to OpenTofu Core go through the public RFC process, including those proposed by the core team. Thus, if you'd like to propose such a change, please prepare an RFC, so that the community can discuss the change and everybody has a chance to voice their opinion. You're also welcome to voice your own opinion on existing RFCs! You can find them by [going to the issues view and filtering by the rfc label](https://github.com/opentofu/opentofu/issues?q=is%3Aopen+is%3Aissue+label%3Arfc).

Generally, we appreciate external contributions very much and would love to work with you on them. **However, please make sure to read the [Contributing a Code Change](#contributing-a-code-change) section prior to making a contribution.**

---

## Core Team

The Core Team consists of the individuals in the [MAINTAINERS](MAINTAINERS) file. This team exists as stewards of OpenTofu: to triage issues, to implement features, to help with and review community contributions, and to communicate with the Technical Steering Committee.

### Issue Triaging

As issues are filed in the OpenTofu project, they go through a processes driven by the Core Team.  This process is in place to prevent duplicate work and to ensure that a discussion happens before work is contributed to avoid frustration.

Steps:
* Issue is filed and given the `pending-decision` label and an additional label to identify their type (`bug`, `enhancement`, `rfc`).
* Issue will first be discussed between the Core Team and the community to iron out any missing details.
* Once the Issue is well understood, the Core Team may decide to accept it, reject it, or pass the decision along to the [Technical Steering Committee](https://github.com/opentofu/opentofu/blob/main/TSC_SUMMARY.md) (`pending-steering-committee-decision`).
  - Occasionally, the Core Team may wait to make a decision to gauge the level of community interest and will add the label `needs-community-input`.
  - To advocate for an issue, give it a reaction and/or add a comment.
* If Accepted:
  - It will have the `pending-decision` label removed and the `accepted` label added.
  - The Core Team may assign one of their members to work on it or may wait for a community member to ask for it to be assigned to them.
  - It may sometimes be labeled with `help-wanted` or `good-first-issue` when the Core Team hopes that someone in the community will be able to pitch in and help on it.
* If Rejected:
  - Not all issues will make it into OpenTofu, but the decision process should be clear and documented.
  - The issue will be closed.


---

<!-- MarkdownTOC autolink="true" -->

- [Contributing a Code Change](#contributing-a-code-change)
- [Working on the Code](#working-on-the-code)
- [Adding or updating dependencies](#adding-or-updating-dependencies)
- [Acceptance Tests: Testing interactions with external services](#acceptance-tests-testing-interactions-with-external-services)
- [Generated Code](#generated-code)

<!-- /MarkdownTOC -->

## Contributing a Code Change

In order to contribute a code change, you should fork the repository, make your changes, and then submit a pull request. Crucially, all code changes should be preceded by an issue that you've been assigned to. If an issue for the change you'd like to introduce already exists, please communicate in the issue that you'd like to take ownership of it. If an issue doesn't yet exist, please create one expressing your interest in working on it and discuss it first, prior to working on the code. Code changes without a related issue will generally be rejected.

**⚠️ Important:** Please avoid working on features or refactor without [an `accepted` issue](https://github.com/opentofu/opentofu/issues?q=is%3Aopen+is%3Aissue+label%3Aaccepted). OpenTofu is a large and complex project and every change needs careful consideration. We cannot merge non-bug pull requests without first having a discussion about them, no matter how trivial the issue may seem.

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

If you wish to work on the OpenTofu CLI source code, you'll first need to install the [Git](https://git-scm.com/) version control system. Use Git to clone this repository into a location of your choice. OpenTofu uses [Go Modules](https://blog.golang.org/using-go-modules), and so you should *not* clone it inside your `GOPATH`.

After that, you can either install the [Go](https://golang.org/) compiler locally, or use [Docker](https://www.docker.com/) to build in a container. At this time the OpenTofu development environment targets only Linux and MacOS systems. While OpenTofu itself is compatible with Windows, unfortunately the unit test suite currently contains Unix-specific assumptions around maximum path lengths, path separators, etc. This means that using Docker is the best option if working on Windows.

If using Visual Studio Code or Goland/IntelliJ, a [devcontainer](.devcontainer.json) is included which integrates directly with Docker. In Visual Studio Code, you can install the [Remote Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers) extension, then reopen the project to get a prompt about activating the devcontainer. In Goland/Intellij, open the `.devcontainers.json` file and click the purple cube icon that appears next to the line numbers to activate the dev container. At this point you can proceed as if [building natively](#building-natively) on Linux.

If not using the devcontainer (if using an incompatible IDE), you can still still build with Docker (and thus need not install any dependencies locally) by running docker commands directly. See the [Building with Docker](#building-with-docker) section.

### Building Natively

Refer to the file [`.go-version`](.go-version) to see which version of Go OpenTofu is currently built with. Other versions will often work, but if you run into any build or testing problems please try with the specific Go version indicated. You can optionally simplify the installation of multiple specific versions of Go on your system by installing [`goenv`](https://github.com/syndbg/goenv), which reads `.go-version` and automatically selects the correct Go version.

### Build with Go

Switch into the root directory of the cloned repository and build OpenTofu using the Go toolchain in the standard way:

```sh
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

### Building with Docker

The easiest way to get started with Docker on Windows and MacOS is using [Docker Desktop](https://www.docker.com/products/docker-desktop/), though other solutions exist. On Linux, follow the steps to [install Docker Engine](https://docs.docker.com/engine/install/) and run the [post-installation steps](https://docs.docker.com/engine/install/linux-postinstall/). Then to build, run:

```sh
docker run --rm -v "$PWD":/usr/src/opentofu -w /usr/src/opentofu golang:1.20.7 GOOS=linux GOARCH=amd64 go build -v -buildvcs=false .
```

Replace the values for `GOOS` snd `GOARCH` with those of your preferred target, e.g. `GOOS=windows` to build a Windows binary.

This will create the `opentofu` binary in the current working directory, which you can run with `./opentofu --version` or [move it to $PATH](https://ubuntuforums.org/showthread.php?t=1056425) for Linux to find it running just `opentofu --version`.

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

### Integration Tests: Testing interactions with external backends

OpenTofu supports various [backends](https://opentofu.org/docs/language/settings/backends/configuration). We run integration test against them to ensure no side effects when using OpenTofu.

Execute to list all available commands to run tests:

```commandline
make list-integration-tests
```

From the list of output commands, you can execute those which involve backends you intend to test against.

For example, execute the command to run integration tests with s3 backend:

```commandline
make test-s3
```

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
