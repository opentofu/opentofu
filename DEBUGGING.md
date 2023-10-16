# Debugging the OpenTofu code

There are various ways to debug the OpenTofu code. There is no ultimate "right" answer, this document intends to collect some of those ways. The order of debugging techniques is completely random.

If you would like to contribute to this debugging guide, please [create a GitHub issue](https://github.com/opentofu/opentofu/issues/new/choose) and propose the enhancement. After that, you can create a pull request and reference this issue in your PR.

For further information on contributing to the code, please refer to the [CONTRIBUTING.md](./CONTRIBUTING.md) file.

<!-- TOC -->

- [Using the debug-opentofu script](#using-the-debug-opentofu-script)
- [Using spew](#using-spew)
- [Using VsCode](#using-vscode)

<!-- /TOC -->

## Using the debug-opentofu script

[debug-opentofu](./scripts/debug-opentofu) is a helper script to launch OpenTofu inside the ["dlv" debugger](https://github.com/go-delve/delve), configured to await a remote debugging connection on port 2345. For more details on how to use this script, please refer to the documentation at the beginning of this script.

## Using spew

[Go-spew](https://github.com/davecgh/go-spew) implements a deep pretty printer for Go data structures to aid in debugging. If you prefer to use println debugging, `spew.Dump` might be helpful.

For more documentation on how to use spew, you can visit the [spew GoDoc site](https://pkg.go.dev/github.com/davecgh/go-spew/spew).

## Using VsCode

Visual Studio Code (VS Code) features native [debugging](https://code.visualstudio.com/docs/editor/debugging) support with the [Go extension](https://marketplace.visualstudio.com/items?itemName=golang.go).

An example [.vscode/launch.json](.vscode/launch.json) configuration file that implements `tofu init` and `tofu plan`:
```json
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
    ]
}
```
