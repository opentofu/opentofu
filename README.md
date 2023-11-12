# OpenTofu

- [Manifesto](https://opentofu.org/manifesto)
- [About the OpenTofu fork](https://opentofu.org/fork)
- [How to install](https://opentofu.org/docs/intro/install)
- [Join our Slack community!](https://opentofu.org/slack)
- [Weekly OpenTofu Status Updates](WEEKLY_UPDATES.md)

![](https://raw.githubusercontent.com/opentofu/brand-artifacts/main/full/transparent/SVG/on-dark.svg#gh-dark-mode-only)
![](https://raw.githubusercontent.com/opentofu/brand-artifacts/main/full/transparent/SVG/on-light.svg#gh-light-mode-only)

**Important Note: This repository is currently a work in progress while we're preparing it for the first stable release and fine-tuning the community contribution process. Please read the [announcement post](https://opentofu.org/fork) for important context and the [contributing docs](CONTRIBUTING.md) for instructions on how to contribute.**

OpenTofu is an OSS tool for building, changing, and versioning infrastructure safely and efficiently. OpenTofu can manage existing and popular service providers as well as custom in-house solutions.

The key features of OpenTofu are:

- **Infrastructure as Code**: Infrastructure is described using a high-level configuration syntax. This allows a blueprint of your datacenter to be versioned and treated as you would any other code. Additionally, infrastructure can be shared and re-used.

- **Execution Plans**: OpenTofu has a "planning" step where it generates an execution plan. The execution plan shows what OpenTofu will do when you call apply. This lets you avoid any surprises when OpenTofu manipulates infrastructure.

- **Resource Graph**: OpenTofu builds a graph of all your resources, and parallelizes the creation and modification of any non-dependent resources. Because of this, OpenTofu builds infrastructure as efficiently as possible, and operators get insight into dependencies in their infrastructure.

- **Change Automation**: Complex changesets can be applied to your infrastructure with minimal human interaction. With the previously mentioned execution plan and resource graph, you know exactly what OpenTofu will change and in what order, avoiding many possible human errors.

## Developing OpenTofu

This repository contains OpenTofu Core, which includes the command line interface and the main graph engine.

- To learn more about compiling OpenTofu and contributing suggested changes, refer to [the contributing guide](CONTRIBUTING.md).

- To submit bug reports or enhancement requests, refer to the [contributing guide](CONTRIBUTING.md) as well.

## Reporting security vulnerabilities
If you've found a vulnerability or a potential vulnerability in OpenTofu please follow [Security Policy](https://github.com/opentofu/opentofu/security/policy). We'll send a confirmation email to acknowledge your report, and we'll send an additional email when we've identified the issue positively or negatively.

## License

[Mozilla Public License v2.0](https://github.com/opentofu/opentofu/blob/main/LICENSE)
