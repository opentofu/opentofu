# OpenTF

- Manifesto: https://opentf.org
- About the OpenTF fork: https://opentf.org/fork
- [Join our Slack community!](https://join.slack.com/t/opentfcommunity/shared_invite/zt-22ifsm1t2-AF6cL0cOdzivP8E~4deDJA)

<img alt="OpenTF" src="https://raw.githubusercontent.com/opentffoundation/brand-artifacts/main/full/transparent/SVG/on-light.svg" width="600px">

**Important Note: This repository is currently a work in progress while we're preparing it for the first alpha release and fine-tuning the community contribution process. Please read the [announcement post](https://opentf.org/fork) for important context and the [contributing docs](CONTRIBUTING.md) for instructions on how to contribute. Additionally, please be mindful that building this repository in its current state and running it might put you in violation of the [Terraform Registry ToS](https://web.archive.org/web/https://registry.terraform.io/terms), if that's where you fetch your providers or modules from.**

OpenTF is an OSS tool for building, changing, and versioning infrastructure safely and efficiently. OpenTF can manage existing and popular service providers as well as custom in-house solutions.

The key features of OpenTF are:

- **Infrastructure as Code**: Infrastructure is described using a high-level configuration syntax. This allows a blueprint of your datacenter to be versioned and treated as you would any other code. Additionally, infrastructure can be shared and re-used.

- **Execution Plans**: OpenTF has a "planning" step where it generates an execution plan. The execution plan shows what OpenTF will do when you call apply. This lets you avoid any surprises when OpenTF manipulates infrastructure.

- **Resource Graph**: OpenTF builds a graph of all your resources, and parallelizes the creation and modification of any non-dependent resources. Because of this, OpenTF builds infrastructure as efficiently as possible, and operators get insight into dependencies in their infrastructure.

- **Change Automation**: Complex changesets can be applied to your infrastructure with minimal human interaction. With the previously mentioned execution plan and resource graph, you know exactly what OpenTF will change and in what order, avoiding many possible human errors.

## Developing OpenTF

This repository contains OpenTF Core, which includes the command line interface and the main graph engine.

- To learn more about compiling OpenTF and contributing suggested changes, refer to [the contributing guide](CONTRIBUTING.md).

- To submit bug reports or enhancement requests, refer to the [contributing guide](CONTRIBUTING.md) as well.

## License

[Mozilla Public License v2.0](https://github.com/opentffoundation/opentf/blob/main/LICENSE)
