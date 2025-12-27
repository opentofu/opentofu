# OpenTofu + ORAS Backend

> 🍴 **This is a fork of [opentofu/opentofu](https://github.com/opentofu/opentofu)** that adds an **ORAS backend** for storing OpenTofu state in OCI registries.

[![Release](https://img.shields.io/github/v/release/vmvarela/opentofu?label=Latest%20Release&style=flat-square)](https://github.com/vmvarela/opentofu/releases/latest)
[![OpenTofu Base](https://img.shields.io/badge/Based%20on-OpenTofu-blue?style=flat-square)](https://github.com/opentofu/opentofu)

---

## 🤖 Why This Fork Exists

This fork is maintained **independently** because:

1. **AI-Generated Code**: The ORAS backend implementation was developed with AI assistance (GitHub Copilot). The upstream OpenTofu project has a [strict policy against AI-generated code](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md) due to licensing concerns with Terraform's BSL license.

2. **Experimental Backend**: OpenTofu historically avoids adding new remote state backends to the core project. This fork serves as a reference implementation and a usable solution for those who want OCI registry state storage.

This fork stays synchronized with upstream releases, allowing you to benefit from all OpenTofu improvements while having access to the ORAS backend.

---

## 📦 What This Fork Adds

This fork includes an **ORAS backend** that allows you to store OpenTofu state in any OCI-compatible container registry (GitHub Container Registry, Amazon ECR, Azure ACR, Google GCR, Docker Hub, Harbor, etc.).

### Key Features

| Feature | Description |
|---------|-------------|
| **OCI Registry Storage** | Store state as OCI artifacts in your existing container registry |
| **Reuse Existing Auth** | Uses Docker credentials and `tofu login` tokens |
| **Distributed Locking** | Lock state to prevent concurrent modifications |
| **State Versioning** | Keep history of state versions with configurable retention |
| **Compression** | Optional gzip compression for state files |
| **Encryption Compatible** | Works with OpenTofu's client-side state encryption |

### Quick Start

```hcl
terraform {
  backend "oras" {
    repository = "ghcr.io/your-org/tf-state"
  }
}
```

### Full Example (with versioning + encryption)

```hcl
terraform {
  backend "oras" {
    repository  = "ghcr.io/your-org/tf-state"
    compression = "gzip"

    versioning {
      enabled      = true
      max_versions = 10
    }
  }

  encryption {
    key_provider "pbkdf2" "main" {
      passphrase = var.state_passphrase
    }
    method "aes_gcm" "main" {
      key_provider = key_provider.pbkdf2.main
    }
    state {
      method = method.aes_gcm.main
    }
  }
}
```

### 📚 Full Documentation

See the [ORAS Backend README](internal/backend/remote-state/oras/README.md) for complete documentation including:
- All configuration parameters
- Authentication setup
- Locking behavior
- Versioning and retention
- Troubleshooting

---

## 🔄 Release Versioning

This fork follows OpenTofu releases with an `-oci` suffix:

| OpenTofu Release | This Fork |
|------------------|-----------|
| `v1.12.0` | `v1.12.0-oci` |
| `v1.11.1` | `v1.11.1-oci` |

This allows you to choose which OpenTofu version you want with ORAS support.

---

## 📥 Installation

### Quick Install (Linux/macOS)

```bash
curl -sSL https://raw.githubusercontent.com/vmvarela/opentofu/develop/install.sh | sh
```

### Quick Install (Windows PowerShell)

```powershell
irm https://raw.githubusercontent.com/vmvarela/opentofu/develop/install.ps1 | iex
```

This installs the binary as `tofu-oras` to avoid conflicts with the official `tofu` installation.

#### Installation Options

**Linux/macOS:**
```bash
# Install specific version
TOFU_ORAS_VERSION=v1.12.0-oci curl -sSL https://raw.githubusercontent.com/vmvarela/opentofu/develop/install.sh | sh

# Install to custom directory
TOFU_ORAS_INSTALL_DIR=~/.local/bin curl -sSL https://raw.githubusercontent.com/vmvarela/opentofu/develop/install.sh | sh

# Install with custom binary name
TOFU_ORAS_BINARY_NAME=tofu curl -sSL https://raw.githubusercontent.com/vmvarela/opentofu/develop/install.sh | sh
```

**Windows PowerShell:**
```powershell
# Install specific version
$env:TOFU_ORAS_VERSION = "v1.12.0-oci"
irm https://raw.githubusercontent.com/vmvarela/opentofu/develop/install.ps1 | iex

# Install to custom directory
$env:TOFU_ORAS_INSTALL_DIR = "$env:USERPROFILE\.local\bin"
irm https://raw.githubusercontent.com/vmvarela/opentofu/develop/install.ps1 | iex
```

### Manual Download

Download the binary for your platform from the [Releases](https://github.com/vmvarela/opentofu/releases) page.

### Build from Source

```bash
git clone https://github.com/vmvarela/opentofu.git
cd opentofu
go build -o tofu-oras ./cmd/tofu
```

---

# OpenTofu (Original Project)

> The following is the original OpenTofu README.

- [HomePage](https://opentofu.org/)
- [How to install](https://opentofu.org/docs/intro/install)
- [Join our Slack community!](https://opentofu.org/slack)

![](https://raw.githubusercontent.com/opentofu/brand-artifacts/main/full/transparent/SVG/on-dark.svg#gh-dark-mode-only)
![](https://raw.githubusercontent.com/opentofu/brand-artifacts/main/full/transparent/SVG/on-light.svg#gh-light-mode-only)

[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/10508/badge)](https://www.bestpractices.dev/projects/10508)

OpenTofu is an OSS tool for building, changing, and versioning infrastructure safely and efficiently. OpenTofu can manage existing and popular service providers as well as custom in-house solutions.

The key features of OpenTofu are:

- **Infrastructure as Code**: Infrastructure is described using a high-level configuration syntax. This allows a blueprint of your datacenter to be versioned and treated as you would any other code. Additionally, infrastructure can be shared and re-used.

- **Execution Plans**: OpenTofu has a "planning" step where it generates an execution plan. The execution plan shows what OpenTofu will do when you call apply. This lets you avoid any surprises when OpenTofu manipulates infrastructure.

- **Resource Graph**: OpenTofu builds a graph of all your resources, and parallelizes the creation and modification of any non-dependent resources. Because of this, OpenTofu builds infrastructure as efficiently as possible, and operators get insight into dependencies in their infrastructure.

- **Change Automation**: Complex changesets can be applied to your infrastructure with minimal human interaction. With the previously mentioned execution plan and resource graph, you know exactly what OpenTofu will change and in what order, avoiding many possible human errors.

## Getting help and contributing

- Have a question?
  - Post it in [GitHub Discussions](https://github.com/orgs/opentofu/discussions)
  - Open a [GitHub issue](https://github.com/opentofu/opentofu/issues/new/choose)
  - Join the [OpenTofu Slack](https://opentofu.org/slack/)!
- Want to contribute?
  - Please read the [Contribution Guide](CONTRIBUTING.md).
- Recurring Events
  - [Community Meetings](https://meet.google.com/xfm-cgms-has) on Wednesdays at 12:30 UTC at this link: https://meet.google.com/xfm-cgms-has ([📅 calendar link](https://calendar.google.com/calendar/event?eid=NDg0aWl2Y3U1aHFva3N0bGhyMHBhNzdpZmsgY18zZjJkZDNjMWZlMGVmNGU5M2VmM2ZjNDU2Y2EyZGQyMTlhMmU4ZmQ4NWY2YjQwNzUwYWYxNmMzZGYzNzBiZjkzQGc))
  - [Technical Steering Committee Meetings](https://meet.google.com/cry-houa-qbk) every other Tuesday at 4pm UTC at this link: https://meet.google.com/cry-houa-qbk ([📅 calendar link](https://calendar.google.com/calendar/u/0/event?eid=M3JyMWtuYWptdXI0Zms4ZnJpNmppcDczb3RfMjAyNTA1MjdUMTYwMDAwWiBjXzNmMmRkM2MxZmUwZWY0ZTkzZWYzZmM0NTZjYTJkZDIxOWEyZThmZDg1ZjZiNDA3NTBhZjE2YzNkZjM3MGJmOTNAZw))

> [!TIP]
> For more OpenTofu events, subscribe to the [OpenTofu Events Calendar](https://calendar.google.com/calendar/embed?src=c_3f2dd3c1fe0ef4e93ef3fc456ca2dd219a2e8fd85f6b40750af16c3df370bf93%40group.calendar.google.com)!

## Reporting security vulnerabilities
If you've found a vulnerability or a potential vulnerability in OpenTofu please follow [Security Policy](https://github.com/opentofu/opentofu/security/policy). We'll send a confirmation email to acknowledge your report, and we'll send an additional email when we've identified the issue positively or negatively.

## Reporting possible copyright issues

If you believe you have found any possible copyright or intellectual property issues, please contact liaison@opentofu.org. We'll send a confirmation email to acknowledge your report.

## Registry Access

In an effort to comply with applicable sanctions, we block access from specific countries of origin.

## License

[Mozilla Public License v2.0](https://github.com/opentofu/opentofu/blob/main/LICENSE)

