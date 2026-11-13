# Local Execution

## Summary

This document defines the configuration and execution model for running OpenTofu providers by provdiing a command and a set of arguments, enabling local development, package manager distribution, and flexible deployment patterns.

## Configuration Model

### Design Philosophy: Familiarity Over Innovation

**Key Principle**: Use existing, familiar configuration structures rather than introducing new syntax. Users already understand the `required_providers` block from years of Terraform/OpenTofu usage. This RFC extends that familiar pattern rather than creating entirely new configuration mechanisms.

### Relationship to Other Proposals

This RFC is one of several approaches being explored for flexible provider execution, the combination of the two links below explore a different approach and is a recommended read before going into this document:

- **[Local-exec providers (PR #3027)](https://github.com/opentofu/opentofu/pull/3027)**: Explores opt-in local execution with different configuration patterns
- **[Registry in a file (PR #2892)](https://github.com/opentofu/opentofu/pull/2892)**: Proposes `.opentofu.deps.hcl` for source mapping and local overrides

This RFC focuses on extending the existing `required_providers` syntax to be more flexible, while the other proposals introduce new configuration files or mechanisms. Each approach has trade-offs that should be discussed. In an ideal world we would find a way to introduce both approaches and this proposal does not block the concept of a Registry in a file.

### Basic Configuration

Providers can be configured using the familiar `required_providers` block, extended with `cmd` and `args` fields:

```hcl
terraform {
  required_providers {
    # Traditional registry-based provider (unchanged)
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    
    # Local script execution (new capability)
    governance = {
      cmd  = "python3"
      args = ["./governance-provider.py", "--stdio"]
    }
    
    # Package manager execution (new capability)
    custom = {
      cmd  = "npx"
      args = ["-y", "@mycompany/provider@1.2.3", "--stdio"]
    }

    # Docker container execution (new capability)
    scanner = {
      cmd  = "docker"
      args = ["run", "--rm", "-i", "security-scanner:latest"]
    }
  }
}
```

The same `required_providers` block supports both traditional (`source`+`version`) and local (`cmd`+`args`) providers, allowing mixed usage patterns and gradual adoption. Usage of both sets of variables should result in an error.

### Advanced Configuration (Possible extension)

For cases requiring environment customization, the `env` field provides environment variable overrides:

```hcl
terraform {
  required_providers {
    governance = {
      cmd  = "python3"
      args = ["./governance-provider.py", "--stdio"]
      env = {
        GOVERNANCE_API_KEY = var.api_key
        GOVERNANCE_LOG_LEVEL = "debug"
        PATH = "${env.PATH}:/opt/custom/bin"
      }
    }
  }
}
```

This will override environment variables from the parent process and pass a set of env vars that is the result of combining the parent process env vars with the set in the block above, preferring the env vars defined in configuration.

## Working Directory

### Default Behavior

**The provider process working directory is set to the directory containing the Terraform configuration file**, not the directory where `tofu` was invoked. This ensures:

1. **Predictable relative paths**: `./provider.py` always relative to .tf file
2. **Consistency**: Same behavior regardless of where `tofu` is run
3. **Module support**: Providers in modules run in the module directory

### Example Layout

```bash
# Directory structure:
/project/
  main.tf          # Has provider config with cmd="./provider.py"
  provider.py      # The actual provider script
  modules/
    custom/
      main.tf      # Child Module has provider config with cmd="./local-provider"
      local-provider
```

### Working Directory for Complex Configurations

For root module configurations, the working directory is the root module directory. For child modules with their own provider configurations, the working directory is the child module directory.

### Version Management

In the case of local providers, OpenTofu assumes that versioning is handled externally by another package manager or versioning system.
For example:

- NPX: `@company/provider@1.2.3` - exact version
- Docker: `image:tag` - image tags
- Local scripts: No versioning (use version control)