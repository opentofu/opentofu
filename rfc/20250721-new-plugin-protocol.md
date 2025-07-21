# RFC: OpenTofu Plugin Protocol (Codename: Seasonings)

> [!NOTE]
> This RFC is **not** proposing to replace providers, modules, or the existing plugin ecosystem. Instead, it aims to **extend** OpenTofu's functionality by providing additional extension points that complement the current system. Existing providers will continue to work exactly as they do today.

## Summary

This RFC proposes a new plugin protocol for OpenTofu (codename "Seasonings") that focuses specifically on the wire protocol and communication layer. This proposal builds upon ideas from two recent RFCs:
- The middleware system proposal ([PR #3016](https://github.com/opentofu/opentofu/pull/3016)) which demonstrated the viability of stdio-based JSON-RPC communication
- @apparentlymart's local-exec providers concept ([PR #3027](https://github.com/opentofu/opentofu/pull/3027)) which envisions a more flexible, unified system for executing providers

**Scope of this RFC**: This document focuses exclusively on:
- The wire protocol design using msgpack over stdio
- Language-agnostic plugin communication patterns
- Type serialization for OpenTofu's cty type system
- Protocol extensibility for future capabilities

**Out of scope** (to be addressed in separate RFCs):
- Plugin discovery and installation mechanisms
- Registry integration and distribution
- Security model and sandboxing
- Specific implementations of middleware, state storage, or provider plugins
- Migration strategies from existing protocols

The goal is to establish a solid foundation for language-agnostic plugin development that can support various plugin types while maintaining OpenTofu's type safety and feature richness.

## Motivation

The current OpenTofu plugin ecosystem has a critical bottleneck: **the Terraform Plugin SDK/Framework is the only actively maintained path to building extensions, and it's locked to Go**.

While the underlying protocol uses gRPC and protobuf, virtually no one interacts with these directly. The reality is:

1. **Go-only ecosystem**: The Plugin SDK/Framework requires Go expertise, excluding developers who work in other languages
2. **Heavyweight for simple tasks**: Adding a simple provider-defined function? A full provider is required. The SDK wasn't designed for these lightweight use cases.

### The Cost of Go Lock-in

Consider who we're excluding from the ecosystem:
- **Security teams** who may work primarily in Python
- **Cost management teams** using data science stacks (Python/R)
- **Platform teams** with existing TypeScript/JavaScript tooling
- **SRE teams** who just need simple Bash script integrations

These teams currently resort to:
- Parsing plan JSON files (missing critical runtime context)
- Writing wrapper scripts around `tofu` commands
- Building complex CI/CD pipelines for what should be simple validations
- Maintaining separate tools that duplicate OpenTofu's resource understanding

### Why Language Agnosticism Matters

The proposed protocol (codename "Seasonings") addresses this by:
- Using stdio communication - available in every programming language
- Adopting msgpack for efficient, typed serialization
- Providing a simple message-based protocol that's easy to implement
- Supporting different capability levels - from simple hooks to full providers
- Maintaining full compatibility with OpenTofu's rich type system

This isn't about replacing the Plugin SDK - it's about opening the door to extensions that the current system wasn't designed to support.

## Detailed Design

### Protocol Overview

The Seasonings protocol uses [msgpack-rpc](https://github.com/msgpack-rpc/msgpack-rpc/blob/master/spec.md) communicated over standard input/output (stdio) with message framing.

#### Why msgpack-rpc?

**msgpack-rpc** provides everything we need:
- Simple, efficient message format using MessagePack encoding
- Request/response correlation via message IDs
- Support for notifications (fire-and-forget messages)
- Proven by successful adoption in projects like Neovim
- Existing libraries in many programming languages

**MessagePack** is **required** (not just nice-to-have) because JSON cannot properly represent OpenTofu's type system:
- **Unknown values**: JSON has no way to represent `cty.UnknownVal`, which is essential during planning when values aren't yet known
- **Type information loss**: JSON converts lists, sets, and tuples all to arrays; maps and objects both to objects - losing critical type distinctions
- **Dynamic types**: OpenTofu uses `cty.DynamicPseudoType` for provider-defined schemas, which requires type information to be preserved alongside values
- **Null vs absent**: JSON cannot distinguish between a null value and an absent value, both critical states in OpenTofu

OpenTofu already uses msgpack internally (via the `ctymsgpack` library from `github.com/zclconf/go-cty`) specifically because these limitations make JSON unsuitable for provider communication.

#### msgpack-rpc Message Format

The msgpack-rpc specification defines three message types:

```
Request:  [type(0), msgid, method, params]
Response: [type(1), msgid, error, result]  
Notification: [type(2), method, params]
```

Example messages (shown as JSON for readability, but sent as msgpack arrays):
```javascript
// Request - actually sent as: [0, 1, "middleware/prePlan", {"resources": [...]}]
[
  0,                          // Message type 0 = Request
  1,                          // Message ID for correlation
  "middleware/prePlan",       // Method name
  {                           // Parameters
    "resources": [...],
    "config": {...}
  }
]

// Response - actually sent as: [1, 1, null, {"warnings": [...]}]
[
  1,                          // Message type 1 = Response
  1,                          // Message ID (matches request)
  null,                       // Error (null = no error)
  {                           // Result
    "warnings": ["Resource cost exceeds $1000/month"]
  }
]

// Notification - actually sent as: [2, "log", {"level": "info", ...}]
[
  2,                          // Message type 2 = Notification
  "log",                      // Method name
  {                           // Parameters
    "level": "info",
    "message": "Processing resource aws_instance.example"
  }
]
```

#### Why stdio?

Standard input/output is the universal IPC mechanism:
- Every language can read stdin and write stdout
- No network ports or sockets to manage
- Works everywhere - containers, restricted environments, etc.
- Simple to debug and test

#### Message Transport

Following Neovim's proven approach, we don't need explicit framing. MessagePack is self-delimiting - each value contains enough information to determine where it ends. This makes the protocol simpler:

1. **Writing**: Encode msgpack-rpc arrays directly to stdout
2. **Reading**: Use a streaming msgpack parser that:
    - Reads bytes from stdin into a buffer
    - Feeds them to the msgpack unpacker
    - Emits complete messages as they're decoded
    - Handles partial messages automatically

This approach has several advantages:
- **Simplicity**: No framing headers to implement or parse
- **Proven**: Neovim's large plugin ecosystem demonstrates this works well
- **Library support**: Most msgpack libraries include streaming parsers
- **Resilient**: Parser can recover from corrupted data

Example pseudo-code for reading:
```python
unpacker = msgpack.Unpacker()
while True:
    data = stdin.read(4096)  # Read available bytes
    unpacker.feed(data)
    for message in unpacker:  # Yields complete messages
        process_msgpack_rpc_message(message)
```

### Message Types (Proposed)

Beyond the standard msgpack-rpc message types, we propose the following methods for plugin lifecycle:

1. **Initialization**
    - `init`: Plugin initialization and capability declaration
    - Returns: Plugin metadata and supported capabilities

2. **Plugin-specific methods**
    - Methods vary based on declared capabilities
    - Examples: `middleware/prePlan`, `storage/read`, `functions/call`

3. **Common methods**
    - `shutdown`: Graceful termination
    - `ping`: Health check

### Plugin Types and Capabilities

We propose a unified plugin SDK that supports multiple capability levels. A single plugin can declare multiple capabilities, allowing natural evolution from simple to complex functionality:

#### Level 1: Middleware Capabilities
Read-only hooks into OpenTofu operations such as:
- `hooks.prePlan`, `hooks.postPlan`
- `hooks.preApply`, `hooks.postApply`
- `hooks.preRefresh`, `hooks.postRefresh`
- Access to resource changes and state (read-only)
- Can emit warnings or fail operations
- See Middleware RFC for more information

#### Level 2: State Storage Capabilities
Custom state backend implementation:
- `storage.read`, `storage.write`
- `storage.lock`, `storage.unlock`
- `storage.list` (for workspace support)
- Backend-specific configuration handling
- TODO: Discuss options about a "granular" state storage system

#### Level 3: Provider Functions
Lightweight computation without resource management:
- `functions.getSchema`: Declare available functions
- `functions.call`: Execute a function with parameters
- Pure functions for data transformation, validation, or external API calls
- No state management or side effects

#### Level 4: Resource Management (Future)
Full provider capabilities:
- `resources.getSchema`: Define resources and data sources
- `resources.planChange`, `resources.applyChange`
- `resources.read`, `resources.import`
- Support for managed resources, data sources
- Ephemeral values and deferred actions

### SDK Design Philosophy

The key to community adoption is lowering the barrier to entry. Inspired by the Model Context Protocol (MCP) SDKs and validated by the Middleware RFC, our SDKs should prioritize developer experience and reduce the time taken to create integrations.

Please note that this is all theoretical for the sake of the RFC!
#### Example: Cost Estimator (TypeScript)

From the Middleware RFC, using the server-style API:

```typescript
import { MiddlewareServer, StdioTransport } from '@opentofu/middleware-sdk';

const server = new MiddlewareServer({
  name: "cost-estimator",
  version: "1.0.0"
});

server
  .postPlan(async (params) => {
    const cost = await estimateCost(params.resource_type, params.after);
    return {
      status: cost.monthly > 1000 ? "fail" : "success",
      message: `Estimated cost: $${cost.monthly}/month`,
      metadata: { estimated_cost: cost }
    };
  })
  .planStageCompleted(async (params) => {
    const totalCost = calculateTotalCost(params.resources);
    return {
      status: totalCost > 5000 ? "fail" : "pass",
      message: `Total plan cost: $${totalCost}/month`
    };
  });

await new StdioTransport().connect(server);
```

#### Example: Multi-capability Plugin (TypeScript)

A plugin server can provide multiple capabilities:

```typescript
import { PluginServer, StdioTransport } from '@opentofu/plugin-sdk';
import { S3Client } from '@aws-sdk/client-s3';
import { z } from 'zod';

const s3 = new S3Client({ region: process.env.AWS_REGION });
const bucket = process.env.STATE_BUCKET;

const server = new PluginServer({
    name: "cloud-toolkit",
    version: "2.0.0",
    capabilities: ["storage", "functions"]
});

// State storage handlers
server
    .storageRead(async ({ path }) => {
        const response = await s3.getObject({
            Bucket: bucket,
            Key: `states/${path}`
        });
        return { 
            state: await response.Body?.transformToString() || null 
        };
    })
    .storageWrite(async ({ path, state }) => {
        await s3.putObject({
            Bucket: bucket,
            Key: `states/${path}`,
            Body: state,
            ServerSideEncryption: 'AES256'
        });
        return { success: true };
    })
    .storageLock(async ({ path }) => {
        // Implement DynamoDB-based locking
        return { lockId: crypto.randomUUID() };
    });

// Provider function definitions (inspired by MCP tools)
server
    .function(
        "presigned_url",
        {
            description: "Generate a presigned S3 URL",
            parameters: z.object({
                key: z.string().describe("S3 object key"),
                expires_in: z.number().default(3600).describe("URL expiration in seconds")
            }),
            returns: z.string()
        },
        async ({ key, expires_in }) => {
            return await s3.getSignedUrl('getObject', {
                Bucket: bucket,
                Key: key,
                Expires: expires_in
            });
        }
    )
    .function(
        "list_objects", 
        {
            description: "List objects in the state bucket",
            parameters: z.object({
                prefix: z.string().optional().describe("Filter by key prefix"),
                max_keys: z.number().default(100).describe("Maximum results")
            }),
            returns: z.array(z.string())
        },
        async ({ prefix, max_keys }) => {
            const response = await s3.listObjectsV2({
                Bucket: bucket,
                Prefix: prefix,
                MaxKeys: max_keys
            });
            return response.Contents?.map(obj => obj.Key) || [];
        }
    );

await new StdioTransport().connect(server);
```

The SDK handles all the msgpack-rpc protocol details, streaming parsing, and error handling. Developers just implement their business logic.

## Backwards Compatibility

- Existing tfplugin6 providers continue to work unchanged
- New protocol runs alongside existing system
- Migration path for providers to adopt new protocol
- Feature flags to enable/disable new protocol

## Alternatives Considered

1. **Extend existing gRPC protocol**: Would maintain compatibility but doesn't address core limitations
3. **Pure JSON-RPC**: Cannot properly handle cty type system
4. **WebAssembly plugins**: Too restrictive for system integration use cases

## Open Questions

1. Should we support network transports (TCP/Unix sockets) in addition to stdio?

## References

- [Middleware RFC (PR #3016)](https://github.com/opentofu/opentofu/pull/3016) - @Yantrio
- [Local-exec providers RFC (PR #3027)](https://github.com/opentofu/opentofu/pull/3027) - @apparentlymart
- [MCP (Model Context Protocol)](https://github.com/modelcontextprotocol/specification)
- [MessagePack Specification](https://msgpack.org/)
- [go-cty Type System](https://github.com/zclconf/go-cty)