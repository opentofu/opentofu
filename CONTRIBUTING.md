# Contributing to OpenTofu

Welcome and thank you for wanting to contribute! 

## Get started

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

---

## Writing code for OpenTofu

Eager to get started on coding? Here's the short version:

1. Set up a Go development environment with Git.
2. Pay attention to copyright: write the code yourself, avoid copy/paste. Disable or limit your AI coding assistant.
3. Run the tests with `go test` in the package you are working on.
4. Build OpenTofu by running `go build ./cmd/tofu`.
5. Update [the changelog](CHANGELOG.md).
6. When you commit, use `git commit -s` to sign off your commits.

---

### Setting up your development environment

You can develop OpenTofu on any platform you like. However, we recommend either a Linux (including WSL on Windows) or a MacOS build environment. You will need [Go](https://golang.org/) and [Git](https://git-scm.com/) installed, and we recommend an IDE to help you with code completion and code quality warnings. (We recommend installing the Go version documented in the [.go-version](.go-version) file.)

Alternatively, if you use Visual Studio Code or Goland/IntelliJ and have Docker or Podman installed, you can also use a [devcontainer](.devcontainer.json). In Visual Studio Code, you can install the [Remote Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers), then reopen the project to get a prompt about activating the devcontainer. In Goland/Intellij, open the `.devcontainers.json` file and click the purple cube icon that appears next to the line numbers to activate the dev container. At this point you can proceed as if you were [building natively](#building-natively) on Linux.

---

### Building OpenTofu

There are two ways to build OpenTofu: natively and in a container. Building natively means you are building directly where your code is located without dynamically creating a container. In contrast, building in a container means you are using a build container to run the build process only.

#### Building natively

To build OpenTofu natively, you will need to install Go in the environment you are running in. You can then run the `go build` command from your OpenTofu source directory as follows:

```sh
GOOS=linux GOARCH=amd64 go build -o tofu -v -buildvcs=false ./cmd/tofu
```

This command will produce a `tofu` binary in your current directory, which you can test by running `./tofu --version`.

> [!TIP]
> Replace the `GOOS` and `GOARCH` values with your target platform if you wish to cross-compile.

#### Building in a container

If you have Docker or a compatible alternative installed, you can run the entire build process in a container too:

```sh
docker run \
  --rm \
  -v "$PWD":/usr/src/opentofu\
  -w /usr/src/opentofu golang:1.21.3\
  GOOS=linux GOARCH=amd64 go build -o tofu -v -buildvcs=false ./cmd/tofu
```

This will create the `tofu` binary in the current working directory, which you can test by running `./tofu --version`.

> [!TIP]
> Replace the `GOOS` and `GOARCH` values with your target platform if you wish to cross-compile. 

---

### Running tests

Similar to builds, you can use the `go test` command to run tests. To run the entire test suite, please run the following command in your OpenTofu source directory:

```sh
go test ./...
```

Alternatively, you can also run the `go test` command in the package you are currently working on.

---

### Updating the changelog

We are keeping track of the changes to OpenTofu in the [CHANGELOG.md](CHANGELOG.md) file. Please update it when you add features or fix bugs in OpeTofu.

---

### Signing off your commits

When you contribute code to OpenTofu, we require you to add a [Developer Certificate of Origin](https://developercertificate.org/) sign-off. Please read the DCO carefully before you proceed and only contribute code you have written yourself. Please do not add code that you have not written from scratch yourself without discussing it in the related issue first.

The simplest way to add a sign-off is to use the `-s` command when you commit:

```
git commit -s -m "My commit message"
```

> [!IMPORTANT]
> Make sure your `user.name` and `user.email` setting in Git matches your GitHub settings. This will allow the automated DCO check to pass and avoid delays when merging your PR.

> [!TIP]
> Have you forgotten your sign-off? You can use `git rebase --signoff` to add it after the fact.

---

### A note on copyright

We take copyright and intellectual property very seriously. A few quick rules should help you:

1. You should write most, if not all of your code yourself.
2. Do not copy code from other authors without permission. If you have permission, always add the `Co-authored-by` sign-off to your commits.
3. When you copy/paste code from within the OpenTofu code, always make it explicit where you copied from.
4. When you copy from external sources, always make explicit where your copy is coming from and make sure that their license and copyright attribution requirements are met.
5. Specifically, do not copy from the Terraform repository, or any PRs filed against that repository. Doing so will immediately disqualify your PR from being merged.

> [!WARNING]
> Coding assistants based on large language models (LLMs), such as ChatGPT or GitHub Copilot, are awesome tools to help. However, in the specific case of OpenTofu the training data may include the BSL-licensed Terraform. Since the OpenTofu/Terraform codebase is very specific and LLMs don't have any other training source, they may emit copyrighted code. Please avoid using LLM-based coding assistants as much as possible. If your PR is found to contain code matching Terraform, your PR will be rejected, and you will not be permitted to work on that feature.

---

## Advanced topics

### Acceptance Tests: Testing interactions with external services

The test command above only runs the self-contained tests that run without external services. There are, however, some optional tests in the OpenTofu CLI codebase that *do* interact with external services. We collectively refer to them as "acceptance tests".

You can enable these by setting the environment variable `TF_ACC=1` when running the tests. We recommend focusing only on the specific package you are working on when enabling acceptance tests, both because it can help the test run to complete faster and because you are less likely to encounter failures due to drift in systems unrelated to your current goal:

```
TF_ACC=1 go test ./internal/initwd
```

---

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

---

### Generated Code

Some files in the OpenTofu CLI codebase are generated. In most cases, we update these using `go generate`, which is the standard way to encapsulate code generation steps in a Go codebase.

```
go generate ./...
```

Use `git diff` afterward to inspect the changes and ensure that they are what you expected.

OpenTofu includes generated Go stub code for the OpenTofu provider plugin protocol, which is defined using Protocol Buffers. Because the Protocol Buffers tools are not written in Go and thus cannot be automatically installed using `go get`, we follow a different process for generating these, which requires that you've already installed a suitable version of `protoc`:

```
make protobuf
```

---

### Adding or updating dependencies

If you need to add or update dependencies, you'll have to make sure they use only approved and compatible licenses. The list of these licenses is defined in [`.licensei.toml`](.licensei.toml).

To help verifying this in local development environment and in continuous integration, we use the [licensei](https://github.com/goph/licensei) open source tool.

After modifying `go.mod` or `go.sum` files, you can run it manually with:

```
export GITHUB_TOKEN=changeme
make license-check
```

Note: you need to define the `GITHUB_TOKEN` environment variable to a valid GitHub personal access token, or you will hit rate limiting from the GitHub API which `licensei` uses to discover the licenses of dependencies.

---

### Backporting

All changes to OpenTofu should, by default, go into the `main` branch. When a fix is important enough, it will be backported to a version branch and release when the next minor release rolls around.

Start the backporting process by making sure both the `main` and the target version branch are up-to-date in your working copy. Look up the commit ID of the commit and then switch to your target version branch. Now create a new branch from that version branch:

```sh
git checkout -b backports/ISSUE_NUMBER
```

Now you can cherry-pick the commit in question to your backport branch:

```sh
git cherry-pick -s COMMIT_ID_HERE
```

Finally, create an additional commit to edit the changelog and send in your PR. 

---

## FAQ

### Who decides if a feature will be implemented and how is that decision made?

When you submit an enhancement request, the [core team](#who-is-the-core-team) looks at your issue first. Given the size of the code, adding a new feature is always a careful balancing act. The core team takes the following points into consideration:

1. **Is it possible to implement this on a technical level?**<br />Sometimes, even if a feature would be extremely useful, the state of the codebase doesn't let us do it. 
2. **Does the feature cause more technical debt?**<br />A feature request may hide a larger issue under the hood. Sometimes it is more desirable to resolve the underlying issue instead of implementing the feature in isolation.
3. **Is there someone who would do the work?**<br />The core team doesn't have the capacity to implement everything, so for many issues community contributions are very welcome. Sometimes companies external to OpenTofu decide to dedicate developers for the development of a specific larger feature, which can also weigh in on the decision-making process.
4. **Is there enough capacity on the core team to support a community contributor?**<br />We don't expect contributors to implement the entire feature in isolation, we actively participate with planning, reviews and writing documentation as needed.
5. **Does this feature enable someone to do something new with OpenTofu they were not able to do before?**<br />We prioritize work based on community input and need. An issue with a large number of reactions is more likely to make it into the accepted phase, but if a viable workaround or tool exists, the feature is less likely to be accepted. If a feature is just the integration of a cool technology, but doesn't solve any problems for a large number of people, it will be rejected.

Depending on the core team's review, a feature request can have the following outcomes:

1. **The feature is accepted for development by the core team.** This means that the core team will schedule it for an upcoming release and develop it.
2. **The feature is accepted and open for community contributions.** The core team adds a `help wanted` label and waits for community volunteers to develop it.
3. **More information is needed.** The core team will either add questions in comments, or when there is a deep technical issue to be resolved, call for an RFC to detail a possible implementation.
4. **More community input is needed.** When an issue is, on its surface, valuable, but there is no track record of a large portion of the community needing it, the core team adds the `needs community input` label. If you are interested in the feature and would like to use it, please add a reaction to the issue and add a description on specifically what problem it would solve for you in a comment.
5. **The feature is rejected.** If based on the criteria above it is not feasible to implement the feature, the core team closes the issue with an explanation why it is being closed.
6. **The feature is referred to the Technical Steering Committee**. If the feature requires the commitment of a larger amount of core developer time, has legal implications, or otherwise requires leadership attention, the core team adds the feature to the agenda of the Technical Leadership Committee. Once decided, the TSC records the decision in the [TSC_SUMMARY.md](TSC_SUMMARY.md) file.

---

### What do the labels mean?

- `pending decision`: there is no decision if this issue will be implemented yet. You can show your support for this issue by commenting on it and describing what implementing this issue would solve for you.
- `pending steering committee decision`: the core team has referred this issue to the Technical Steering Committee for a decision.
- `accepted`: the issue is accepted for development by either the core team or a community contributor. Please check if it's assigned to someone and comment on the issue if you want to work on it.
- `help wanted`: this issue is open for community contributions. Please check if it's assigned to someone and comment on the issue if you want to work on it.
- `good first issue`: this issue is relatively simple. If you are looking for a first contribution, this may be for you. Please check if it's assigned to someone and comment on the issue if you want to work on it.
- `bug`: it's broken and needs to be fixed.
- `enhancement`: it's a short-form feature request. If the implementation path is unclear, an `rfc` may be needed in addition.
- `documentation`: something that needs a description on the OpenTofu website.
- `rfc`: a long-form feature request with details on how exactly it should be implemented.
- `question`: the core team needs more information on the issue to decide.
- `needs community input`: the core team needs to see how many people are affected by this issue. You can provide feedback by using reactions on the issue and adding your use case in the comments. (Please describe what problem it would solve for you specifically.)
- `need rfc`: this issue needs a detailed technical description on how it would be implemented in the form of an RFC.

---

### Can you implement X in the language?

It depends. The OpenTofu language is based on the [HCL language](https://github.com/hashicorp/hcl) and the [cty typing system](https://github.com/zclconf/go-cty). Since both are available under an open source license, and we prefer to keep compatibility as much as possible, we currently don't maintain a fork of these libraries. Language features may need to be partially or fully implemented in HCL or cty and if that is the case, we can't implement them in OpenTofu without changes to the respective libraries beforehand.

---

### I don't like HCL, can you replace it?

HCL is baked into every corner of the OpenTofu codebase, so is here to stay. However, you can use the [JSON configuration syntax](https://opentofu.org/docs/language/syntax/json/) to write your code in JSON instead of HCL.

---

### Can you fix a bug in or add a feature to the Hashicorp providers, such as AWS, Azure, etc.?

We currently only maintain a read-only mirror of these providers for the purposes of building binaries for the OpenTofu registry which would otherwise not be available. They are not true downstream versions that we can add patches to and OpenTofu users expect them to work the same as the upstream versions. In short: no, we cannot fix bugs or add features to these providers.

---

### Can you fork project X?

Currently, we are at capacity for development and do not have additional capacity to take on additional projects unless necessary for the continued work on OpenTofu. While the final determination lies with the [Technical Steering Committee](#who-is-the-technical-steering-committee), the answer is likely no in almost all cases.

---

### Who is the core team?

Core team members are full time developers sponsored by the participating companies. You can find the list of core developers in the [MAINTAINERS](MAINTAINERS) file. Their role is to triage issues, work on feature development, help plan, review, and document community contributions, support the community, and refer feature requests to the Technical Steering Committee.

---

### Can I become a core team member?

Possibly, please look for open positions with the sponsoring companies as they hire core team members. The interview process is the same regardless of which sponsoring company you apply to. To become a core team member, you must be equally good at Go and at communication since much of our work is helping the community. Good luck!

---

### Who is the Technical Steering Committee?

The Technical Steering Committee consists of one delegate from each company sponsoring the OpenTofu core team. You can find their names in the [TSC_SUMMARY.md](TSC_SUMMARY.md) file. Their role is to decide on larger commitments of core developer time, as well as long-term strategic issues.
