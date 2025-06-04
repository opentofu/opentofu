# OpenTofu Middleware

This directory contains the OpenTofu middleware SDK and example middleware implementations.

## Structure

- **sdk/** - TypeScript SDK for building OpenTofu middleware
- **cost-estimator/** - Example middleware that provides cloud cost estimation

## SDK

The OpenTofu Middleware SDK provides a TypeScript library for developing middleware with:

- Full type safety for all hooks and parameters
- JSON-RPC protocol handling
- Support for middleware chaining
- Utilities for logging and resource handling
- Transport abstraction (stdio, with potential for HTTP, gRPC, etc.)

### Quick Start

```typescript
import { MiddlewareServer, StdioTransport } from "@opentofu/middleware";

const server = new MiddlewareServer({
  name: "my-middleware",
  version: "1.0.0"
});

server
  .postPlan(async (params) => {
    return {
      status: "pass",
      message: `Processed ${params.resource_type}.${params.resource_name}`
    };
  });

const transport = new StdioTransport();
await transport.connect(server);
```

## Cost Estimator

An example production-ready middleware that demonstrates:

- Multi-cloud cost estimation (AWS, GCP, Azure)
- Resource-specific pricing logic
- Tag-based usage hints
- Middleware chaining support
- Proper error handling and logging

## Development

Each package has its own development setup. Generally:

```bash
cd sdk  # or cost-estimator
npm install
npm run build
npm run typecheck
npm run lint
```

## Creating Your Own Middleware

1. Create a new directory for your middleware
2. Add the SDK as a dependency:
   ```json
   {
     "dependencies": {
       "@opentofu/middleware": "file:../sdk"
     }
   }
   ```
3. Use the SDK to build your middleware logic
4. See the cost-estimator for a complete example

## License

MPL-2.0