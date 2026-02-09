![](https://raw.githubusercontent.com/opentofu/brand-artifacts/main/full/transparent/SVG/on-dark.svg#gh-dark-mode-only)
![](https://raw.githubusercontent.com/opentofu/brand-artifacts/main/full/transparent/SVG/on-light.svg#gh-light-mode-only)

[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/10508/badge)](https://www.bestpractices.dev/projects/10508)

[Homepage](https://opentofu.org/) | [Slack](https://opentofu.org/slack) | [Get Started](https://opentofu.org/docs/intro/install)

OpenTofu is an OSS tool for building, changing, and versioning infrastructure safely and efficiently. OpenTofu can manage existing and popular service providers as well as custom in-house solutions.

## Getting help and contributing

- Have a question?
  - Post it in [GitHub Discussions](https://github.com/orgs/opentofu/discussions)
  - Open a [GitHub issue](https://github.com/opentofu/opentofu/issues/new/choose)
  - Join us in the [#opentofu channel on the CNCF Slack](https://opentofu.org/slack/)!
- Want to contribute?
  - Please read the [Contribution Guide](CONTRIBUTING.md).
- Recurring Events
  - [Community Meetings](https://meet.google.com/xfm-cgms-has) on Wednesdays at 12:30 UTC ([calendar](https://calendar.google.com/calendar/event?eid=NDg0aWl2Y3U1aHFva3N0bGhyMHBhNzdpZmsgY18zZjJkZDNjMWZlMGVmNGU5M2VmM2ZjNDU2Y2EyZGQyMTlhMmU4ZmQ4NWY2YjQwNzUwYWYxNmMzZGYzNzBiZjkzQGc))
  - [Technical Steering Committee Meetings](https://meet.google.com/cry-houa-qbk) every other Tuesday at 4pm UTC ([calendar](https://calendar.google.com/calendar/u/0/event?eid=M3JyMWtuYWptdXI0Zms4ZnJpNmppcDczb3RfMjAyNTA1MjdUMTYwMDAwWiBjXzNmMmRkM2MxZmUwZWY0ZTkzZWYzZmM0NTZjYTJkZDIxOWEyZThmZDg1ZjZiNDA3NTBhZjE2YzNkZjM3MGJmOTNAZw))

> [!TIP]
> For more OpenTofu events, subscribe to the [OpenTofu Events Calendar](https://calendar.google.com/calendar/embed?src=c_3f2dd3c1fe0ef4e93ef3fc456ca2dd219a2e8fd85f6b40750af16c3df370bf93%40group.calendar.google.com)!

## Key features

- **Infrastructure as Code**: Infrastructure is described using a high-level configuration syntax. This allows a blueprint of your datacenter to be versioned and treated as you would any other code. Additionally, infrastructure can be shared and re-used.

- **Execution Plans**: OpenTofu has a "planning" step where it generates an execution plan. The execution plan shows what OpenTofu will do when you call apply. This lets you avoid any surprises when OpenTofu manipulates infrastructure.

- **Resource Graph**: OpenTofu builds a graph of all your resources, and parallelizes the creation and modification of any non-dependent resources. Because of this, OpenTofu builds infrastructure as efficiently as possible, and operators get insight into dependencies in their infrastructure.

- **Change Automation**: Complex changesets can be applied to your infrastructure with minimal human interaction. With the previously mentioned execution plan and resource graph, you know exactly what OpenTofu will change and in what order, avoiding many possible human errors.

## Nightly Builds

Nightly builds are available for testing the latest changes on `main`. These are experimental and not intended for production use. Each build is removed after 30 days.

Nightly builds can be found at `https://nightlies.opentofu.org/nightlies`. For those who want to automate with tooling, `https://nightlies.opentofu.org/nightlies/latest.json` will be kept up to date with the latest build information.

For more details, see [RELEASE.md](RELEASE.md#nightly-builds).

## Reporting security vulnerabilities

If you've found a vulnerability or a potential vulnerability in OpenTofu please follow [Security Policy](https://github.com/opentofu/opentofu/security/policy). We'll send a confirmation email to acknowledge your report, and we'll send an additional email when we've identified the issue positively or negatively.

## Reporting possible copyright issues

If you believe you have found any possible copyright or intellectual property issues, please contact liaison@opentofu.org. We'll send a confirmation email to acknowledge your report.

## Registry Access

In an effort to comply with applicable sanctions, we block access from specific countries of origin. For more details, see the [Registry Inclusion Policy](https://github.com/opentofu/registry/blob/main/POLICY.md).

## License

[Mozilla Public License v2.0](https://github.com/opentofu/opentofu/blob/main/LICENSE)
