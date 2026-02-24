## Writing code for OpenTofu

Eager to get started on coding? Here's the short version:

1. Set up a Go development environment with Git.
2. Pay attention to copyright: [please read the DCO](https://developercertificate.org/), write the code yourself, avoid copy/paste. Disable your AI coding assistant.
3. Run the tests with `go test` in the package you are working on.
4. Build OpenTofu by running `go build ./cmd/tofu`.
5. Update [the changelog](../CHANGELOG.md).
6. When you commit, use `git commit -s` to sign off your commits for the DCO.
7. Submit a PR and complete the checklist included in the template.
8. Your PR will be reviewed by the maintainers once it is marked as ready to review.

---

### Setting up your development environment

You can develop OpenTofu on any platform you like. However, we recommend either a Linux (including WSL on Windows) or a macOS build environment. You will need [Go](https://golang.org/) and [Git](https://git-scm.com/) installed, and we recommend an IDE to help you with code completion and code quality warnings. We recommend installing the latest available version of Go, and then letting the Go toolchain select suitable language and tool versions automatically based on directives in OpenTofu's `go.mod` file.

Alternatively, if you use Visual Studio Code or Goland/IntelliJ and have Docker or Podman installed, you can also use a [devcontainer](../.devcontainer.json). In Visual Studio Code, you can install the [Remote Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers), then reopen the project to get a prompt about activating the devcontainer. In Goland/Intellij, open the `.devcontainers.json` file and click the purple cube icon that appears next to the line numbers to activate the dev container. At this point, you can proceed as if you were [building natively](#building-opentofu) on Linux.

---

### Building OpenTofu

To build OpenTofu, you will need to install Go in the environment you are running in. You can then run the `go build` command from your OpenTofu source directory as follows:

```sh
go build ./cmd/tofu
```

This command will produce a `tofu` binary in your current directory, which you can test by running `./tofu --version`.

> [!TIP]
> Add the `GOOS` and `GOARCH` values with your target platform if you wish to cross-compile. You can find more information in the [Go documentation](https://pkg.go.dev/cmd/go#hdr-Compile_and_run_Go_program).

---

### Running tests

Similar to builds, you can use the `go test` command to run tests. To run the entire test suite, please run the following command in your OpenTofu source directory:

```sh
go test ./...
```

Alternatively, you can also run the `go test` command in the package you are currently working on:

```
go test ./internal/command/...
go test ./internal/addrs
```

> [!TIP]
> You can find more information on testing in the [Go documentation](https://pkg.go.dev/cmd/go#hdr-Test_packages).

---

### Debugging OpenTofu

We recommend using an interactive debugger for finding issues quickly. Most IDE's have a built-in option for this, but you can also set up [dlv](https://github.com/go-delve/delve) on a remote machine for debugging. You can use the [`debug-opentofu`](../scripts/debug-opentofu) script to run OpenTofu in debug mode. You can then connect to the remote machine on port 2345 for debugging.

For VSCode, add the following setting to `.vscode/launch.json` for easy debugging:

```json5
{
    // Use IntelliSense to learn about possible attributes.
    // Hover to view descriptions of existing attributes.
    // For more information, visit: https://go.microsoft.com/fwlink/?linkid=830387
    "version": "0.2.0",
    "configurations": [
        {
            "name": "tofu init",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/cmd/tofu",
            // You can update the environment variables here
            // For more information, visit: https://opentofu.org/docs/cli/config/environment-variables/
            "env": {
                "TF_LOG": "trace"
            },
            // You can update your arguments for init command here
            // Comment out the following line and update your workdir to target
            // "args": ["-chdir=<WORKDIR>", "init"]
            "args": ["init"]
        },
        {
            "name": "tofu plan",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/cmd/tofu",
            "env": {
                "TF_LOG": "trace"
            },
            // You can update your arguments for plan command here
            // Comment out the following line and update your workdir to target
            // "args": ["-chdir=<WORKDIR>", "plan"]
            "args": ["plan"]
        },
        {
            "name": "opentofu test run",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/internal/lang/evalchecks/eval_for_each_test.go",
            // You can update your arguments for go test command here
            // "args": ["-test.run", "TestName/sub_test"]
            // or to run a whole test
            // "args": ["-test.run", "TestName"]
            "args": ["-test.run", "TestEvaluateForEachExpression_errors/set_containing_marked_values"]
        }
    ]
}
```


Similarly, you can add the following configurations to your `.idea/runConfigurations` folder if you use Goland/IntelliJ:

```xml
<!-- .idea/runConfigurations/tofu_init.xml -->
<component name="ProjectRunConfigurationManager">
  <configuration default="false" name="tofu init" type="GoApplicationRunConfiguration" factoryName="Go Application">
    <module name="opentofu" />
    <working_directory value="$PROJECT_DIR$" />
    <parameters value="init" />
    <kind value="DIRECTORY" />
    <package value="github.com/opentofu/opentofu/cmd/tofu" />
    <directory value="$PROJECT_DIR$/cmd/tofu" />
    <filePath value="$PROJECT_DIR$" />
    <method v="2" />
  </configuration>
</component>
```

```xml
<!-- .idea/runConfigurations/tofu_plan.xml -->
<component name="ProjectRunConfigurationManager">
  <configuration default="false" name="tofu plan" type="GoApplicationRunConfiguration" factoryName="Go Application">
    <module name="opentofu" />
    <working_directory value="$PROJECT_DIR$" />
    <parameters value="plan" />
    <kind value="DIRECTORY" />
    <package value="github.com/opentofu/opentofu/cmd/tofu" />
    <directory value="$PROJECT_DIR$/cmd/tofu" />
    <filePath value="$PROJECT_DIR$" />
    <method v="2" />
  </configuration>
</component>
```

In addition to interactive debugging, you can also use [go-spew](https://github.com/davecgh/go-spew) to print complex data structures while running the code.

---

### Signing off your commits

When you contribute code to OpenTofu, we require you to add a [Developer Certificate of Origin](https://developercertificate.org/) sign-off. Please read the DCO carefully before you proceed and only contribute code you have written yourself. Please do not add code that you have not written from scratch yourself without discussing it in the related issue first.

The simplest way to add a sign-off is to use the `-s` command when you commit:

```
git commit -s -m "My commit message"
```

> [!IMPORTANT]
> Make sure your `user.name` and `user.email` settings in Git match your GitHub settings. This will allow the automated DCO check to pass and avoid delays when merging your PR.

> [!TIP]
> Have you forgotten your sign-off? Click the "details" button on the failing DCO check for a guide on how to fix it!

---

### A note on copyright

We take copyright and intellectual property very seriously. A few quick rules should help you:

1. When you submit a PR, you are responsible for the code in that pull request. You signal your acceptance of the [DCO](https://developercertificate.org/) with your sign-off.
2. If you include code in your PR that you didn't write yourself, make sure you have permission from the author. If you have permission, always add the `Co-authored-by` sign-off to your commits to indicate the author of the code you are adding.
3. Be careful about AI coding assistants! Coding assistants based on large language models (LLMs), such as ChatGPT or GitHub Copilot, are awesome tools to help. However, in the specific case of OpenTofu, the training data may include the BSL-licensed Terraform. Since the OpenTofu/Terraform codebase is very specific and LLMs don't have any other training sources, they may emit copyrighted code. Please avoid using LLM-based coding assistants.
4. When you copy/paste code from within the OpenTofu code, always make it explicit where you copied from. This helps us resolve issues later on.
5. Before you copy code from external sources, make sure that the license allows this. Also make sure that any licensing requirements, such as attribution, are met. When in doubt, ask first!
6. Specifically, do not copy from the Terraform repository, or any PRs others have filed against that repository. This code is licensed under the BSL, a license which is not compatible with OpenTofu. (You may submit the same PR to both Terraform and OpenTofu as long as you are the author of both.)

> [!WARNING]
> To protect the OpenTofu project from legal issues violating these rules will immediately disqualify your PR from being merged and you from working on that area of the OpenTofu code base in the future. Repeat violations may get you barred from contributing to OpenTofu. 

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

OpenTofu supports various [backends](https://opentofu.org/docs/language/settings/backends/configuration). We run integration tests against them to ensure no side effects when using OpenTofu.

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

OpenTofu includes generated Go stub code for the Terraform provider plugin protocol, which is defined using Protocol Buffers. Because the Protocol Buffers tools are not written in Go and thus cannot be automatically installed using `go get`, we follow a different process for generating these, which requires that you've already installed a suitable version of `protoc`:

```
make protobuf
```

---

### Adding or updating dependencies

If you need to add or update dependencies, you'll have to make sure they use only approved and compatible licenses. The list of these licenses is defined in [`.licensei.toml`](../.licensei.toml).

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
