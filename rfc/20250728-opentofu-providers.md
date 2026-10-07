# OpenTofu Providers

> [!TIP]
> **Short on time?** Skip to the [TL;DR example](#tldr) to see a quick demonstration of what this RFC enables.

## Summary

This RFC proposes **a new type of provider for OpenTofu** that dramatically lowers the barrier to entry for provider development. These "OpenTofu Providers" will coexist with traditional Terraform providers, using a simplified execution model and offering SDKs in multiple programming languages.

This proposal evolved from my [original middleware RFC concept](https://github.com/opentofu/opentofu/pull/3016). After community discussion and exploration of the broader challenges in provider development, it became clear that the solution warranted a completely new provider type rather than just middleware functionality.

> [!NOTE]  
> This new provider type is intended to exist alongside existing Terraform providers. There is no plan to stop support for Terraform providers in any way.
> The OpenTofu project should do it's best to avoid fragmentation of the ecosystem where possible.

## Composable Architecture

The components defined in this RFC collection are designed to be **composable and modular**. While they work together to create a comprehensive "big picture" solution, each component can theoretically be:

- **Developed independently**: Different teams can work on different components
- **Implemented separately**: Components can be rolled out in phases
- **Replaced with alternatives**: Other proposals or implementations could substitute individual components

**I actively encourage alternative proposals!** Community members are welcome to open RFCs proposing different approaches for any of these components:

- Alternative provider protocols (different from our MessagePack approach)
- Different SDK architectures or language bindings
- Alternative execution models (beyond cmd+args)
- Different registry integration strategies
- Novel approaches to provider extensions

The modular design means that swapping out individual components should have minimal impact on the rest of the system. For example, someone could propose a WebAssembly-based provider protocol while keeping the same SDK patterns, or suggest an entirely different SDK approach while using the same underlying protocol.

This RFC represents **one cohesive proposal** for how these components could work together, but I explicitly designed it to be flexible and evolutionary. The OpenTofu ecosystem benefits from multiple perspectives and approaches.

In order to make this RFC easier to read and implement, I have split it into several focused documents:

### Core Architecture

1. [Provider Protocol](./20250728-opentofu-providers/01-provider-protocol.md) - Defines the stdio-based communication protocol using MessagePack
2. [Provider Client Library](./20250728-opentofu-providers/02-provider-client-library.md) - Multiplexing library that abstracts protocol differences

### Developer Experience

3. [Provider SDK](./20250728-opentofu-providers/03-provider-sdk.md) - Language-specific SDKs with simple, idiomatic APIs
4. [Local Execution](./20250728-opentofu-providers/04-local-execution.md) - Configuration and execution model for cmd+args providers

### Distribution and Discovery

5. [Registry Integration](./20250728-opentofu-providers/05-registry-integration.md) - Registry distribution and discovery mechanisms

### Advanced Features

6. [Provider Extensions](./20250728-opentofu-providers/06-provider-extensions.md) - Extensibility framework and proposed extensions:
   - [Middleware Integration](./20250728-opentofu-providers/06a-middleware.md) - Provider-served middleware for governance and compliance
   - [State Management Enhancements](./20250728-opentofu-providers/06b-state-management.md) - Advanced state handling capabilities

## Background

Current provider development using terraform-plugin-framework presents significant barriers:

- **Language lock-in**: Providers must be written in Go, excluding developers from other ecosystems
- **Complex toolchain**: Requires GoReleaser, GPG signing, GitHub Actions workflows
- **Slow development cycles**: 15+ minute test feedback loops due to integration test requirements
- **Excessive ceremony**: Scaffolding, boilerplate, and complex local testing setup

These barriers prevent many engineers from creating custom providers for governance, internal tooling, or specialized use cases.

## Proposed Solution

OpenTofu Providers introduce:

1. **Multi-language support**: SDKs for TypeScript, Python, Go, and other languages
2. **Simple execution model**: Providers run as local processes and talk over simple protocols and transports
3. **Progressive distribution**: Simplify the process from Local development → package managers → registry publication
4. **Unified interface**: Seamless integration with existing provider ecosystem

## Security Considerations

OpenTofu providers run with the same security model as existing Terraform providers - they execute with the permissions of the user running the `tofu` command and have access to the same system resources.

## TL;DR

This RFC enables you to write OpenTofu providers in any language and run them locally without complex build processes. Here's what it looks like:

**1. Write a simple provider in TypeScript:**
```typescript
// my-provider.ts
import { Provider, StdioTransport } from '@opentofu/provider-sdk';
import { z } from 'zod';

const provider = new Provider({
  name: "myapp",
  version: "1.0.0",
});

provider.resource("user", {
  schema: z.object({
    name: z.string(),
    email: z.string().email(),
    // Computed
    id: z.string().optional(),
  }),
  methods: {
    async create(config) {
      // Call your API to create user
      const user = await api.createUser(config.name, config.email);
      return {
        id: user.id,
        state: { ...config, id: user.id }
      };
    },
    async read(id, config) {
      const user = await api.getUser(id);
      return user ? { ...config, ...user } : null;
    },
    // ... update, delete methods
  }
});

new StdioTransport().connect(provider);
```

**2. Use it in your OpenTofu configuration:**
```hcl
terraform {
  required_providers {
    myapp = {
      cmd  = "npx"
      args = ["tsx", "./my-provider.ts"]
    }
  }
}

resource "myapp_user" "admin" {
  name  = "admin"
  email = "admin@company.com"
}
```

**3. Run OpenTofu normally:**
```bash
tofu plan   # Works just like any other provider
tofu apply  # Creates the user via your Python code
```

**That's it!** No Go toolchain, no complex build processes, no registry submissions required for local development. When you're ready to share, publish to npm/PyPI/etc., or eventually to the OpenTofu registry.

This RFC also includes middleware capabilities, so providers can intercept operations for cost tracking, approval workflows, and governance policies.

## References

- Original Provider Client SDK Discussion: [Issue #3033](https://github.com/opentofu/opentofu/issues/3033)
- Original "Seasonings" Plugin Protocol: [PR #3051](https://github.com/opentofu/opentofu/pull/3051)
- **DRAFT** Local-exec providers: [PR #3027](https://github.com/opentofu/opentofu/pull/3027)
- **DRAFT** Registry in a file: [PR #2892](https://github.com/opentofu/opentofu/pull/2892)
- Original Middleware RFC: [20250711-Middleware-For-Enhanced-Operations.md](20250711-Middleware-For-Enhanced-Operations.md)