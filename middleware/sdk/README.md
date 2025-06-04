# OpenTofu Middleware SDK

A TypeScript library for developing OpenTofu middleware with type safety and better developer experience.

## Installation

```bash
# From the SDK directory
npm install
npm run build
```

## Usage

### Using the Simplified API

```typescript
import { Middleware, FileLogger, ChainUtils, ResourceUtils } from '@opentofu/middleware';

// Create a logger
const logger = new FileLogger('/tmp/my-middleware.log');

// Define your middleware
const middleware = new Middleware({
  name: 'my-middleware',
  capabilities: ['pre-plan', 'post-plan'],
  logger: (msg) => logger.log(msg),
  handlers: {
    'post-plan': async (params) => {
      // Access previous middleware data
      const previousData = params.previous_middleware_metadata || {};
      const chainPosition = ChainUtils.getChainPosition(previousData);
      
      // Log resource info
      const resourceId = ResourceUtils.formatResourceId(
        params.resource_type, 
        params.resource_name
      );
      logger.log(`Processing ${resourceId} at position ${chainPosition}`);
      
      // Return result
      return {
        status: 'pass',
        metadata: {
          middleware: 'my-middleware',
          position: chainPosition,
          timestamp: new Date().toISOString()
        }
      };
    }
  }
});

// Start the middleware
middleware.start();
```

### Using the Server/Transport Pattern

```typescript
import { MiddlewareServer, StdioTransport, FileLogger, ResourceUtils } from '@opentofu/middleware';

// Create middleware server
const server = new MiddlewareServer({
  name: 'my-middleware',
  version: '1.0.0'
});

// Add handlers using fluent API
server
  .postPlan(async (params) => {
    const resourceId = ResourceUtils.formatResourceId(
      params.resource_type,
      params.resource_name
    );
    
    return {
      status: 'pass',
      message: `Processed ${resourceId}`,
      metadata: {
        middleware: 'my-middleware',
        timestamp: new Date().toISOString()
      }
    };
  })
  .preApply(async (params) => {
    // Pre-apply logic
    return { status: 'pass' };
  });

// Create transport and connect
const transport = new StdioTransport();
await transport.connect(server);
```

## Features

- Full TypeScript support with type definitions for all OpenTofu middleware hooks
- Built-in JSON-RPC handling
- Middleware chaining support with access to previous middleware metadata
- Utility classes for logging, chain management, and resource handling
- Async/await support for handlers

## API Reference

### Middleware Class

The main class for creating middleware.

```typescript
new Middleware(options: MiddlewareOptions)
```

### MiddlewareOptions

- `name`: Name of your middleware
- `capabilities`: Array of supported hooks
- `handlers`: Object with handler functions for each hook
- `logger`: Optional logging function

### Available Hooks

- `pre-plan`: Called before planning a resource
- `post-plan`: Called after planning a resource
- `pre-apply`: Called before applying changes
- `post-apply`: Called after applying changes
- `pre-refresh`: Called before refreshing state
- `post-refresh`: Called after refreshing state

### Utility Classes

#### FileLogger
Simple file-based logging utility.

#### ChainUtils
Utilities for working with middleware chains:
- `getMiddlewareData()`: Get data from a specific middleware
- `getChainedMiddleware()`: Get all middleware names in chain
- `hasMiddleware()`: Check if a middleware has run
- `getChainPosition()`: Get position in chain

#### ResourceUtils
Utilities for working with resources:
- `formatResourceId()`: Format resource identifier
- `hasTag()`: Check if resource has a tag
- `getTag()`: Get tag value
- `isAwsResource()`: Check if AWS resource
- `getProviderName()`: Extract provider name