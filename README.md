# OpenTF

- Website: https://www.opentf.org
- Documentation: [https://www.opentf.org/docs/](https://www.opentf.org/docs/)

<img alt="OpenTF" src="https://www.datocms-assets.com/2885/1629941242-logo-terraform-main.svg" width="600px">

OpenTF is an OSS tool for building, changing, and versioning infrastructure safely and efficiently. OpenTF can manage existing and popular service providers as well as custom in-house solutions.

The key features of OpenTF are:

- **Infrastructure as Code**: Infrastructure is described using a high-level configuration syntax. This allows a blueprint of your datacenter to be versioned and treated as you would any other code. Additionally, infrastructure can be shared and re-used.

- **Execution Plans**: OpenTF has a "planning" step where it generates an execution plan. The execution plan shows what Terraform will do when you call apply. This lets you avoid any surprises when OpenTF manipulates infrastructure.

- **Resource Graph**: OpenTF builds a graph of all your resources, and parallelizes the creation and modification of any non-dependent resources. Because of this, OpenTF builds infrastructure as efficiently as possible, and operators get insight into dependencies in their infrastructure.

- **Change Automation**: Complex changesets can be applied to your infrastructure with minimal human interaction. With the previously mentioned execution plan and resource graph, you know exactly what OpenTF will change and in what order, avoiding many possible human errors.

## Developing OpenTF

This repository contains only OpenTF core, which includes the command line interface and the main graph engine. Providers are implemented as plugins, and OpenTF can automatically download providers that are published on [the Terraform Registry](https://registry.terraform.io). For more information, see [Extending Terraform](https://www.terraform.io/docs/extend/index.html).

- To learn more about compiling OpenTF and contributing suggested changes, refer to [the contributing guide](.github/CONTRIBUTING.md).

- To learn more about how we handle bug reports, refer to the [bug triage guide](./BUGPROCESS.md).

- To learn how to contribute to the OpenTF documentation in this repository, refer to the [Terraform Documentation README](/website/README.md).

## License

[Mozilla Public License v2.0](https://github.com/OpenFTFoundation/OpenTF/blob/main/LICENSE)
