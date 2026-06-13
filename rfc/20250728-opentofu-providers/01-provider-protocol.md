# Provider Protocol

## Summary

This document defines the stdio-based communication protocol for OpenTofu providers, using MessagePack-RPC for efficient, language-agnostic communication. The protocol enables the SDK to act as a translator between the methods in `providers.Interface` and the simplified methods that provider developers write.

> [!NOTE]  
> This protocol specification is based on my original [Seasonings plugin protocol RFC](https://github.com/opentofu/opentofu/pull/3051), adapted for the broader OpenTofu provider ecosystem.

## Protocol Overview

The OpenTofu Provider Protocol uses MessagePack-RPC over standard input/output (stdio) streams to enable communication between OpenTofu Core (via the Provider Client Library) and provider implementations. This approach provides:

- **Language agnostic**: Any language with MessagePack support can implement providers
- **Type preservation**: Maintains OpenTofu's complex type system including unknown values
- **Efficient serialization**: Binary format with smaller message sizes than JSON
- **Extensible design**: Protocol can evolve with new message types and capabilities

## Wire Protocol

### RPC Framework Decision

> [!NOTE]  
> I am currenty not sure if we should propose one of two approaches for the RPC framework:
> 1. **MessagePack-RPC**: Using the existing [msgpack-rpc specification](https://github.com/msgpack-rpc/msgpack-rpc), though this project appears unmaintained
> 2. **Custom JSON-RPC-style**: Implementing JSON-RPC semantics but using MessagePack serialization with our own library
> 
> Both approaches use MessagePack for serialization to maintain type fidelity and performance, but differ in message structure and ecosystem support.

### Message Format

The protocol uses MessagePack serialization with RPC semantics. Depending on the final framework decision, messages follow a pattern as similar as possible to the [JSON-RPC 2.0 specification](https://www.jsonrpc.org/specification). This may be json-rpc in msgpack-serialized format, or a wrapper around it to add msgpack on top of json-rpc. undecided.

### Transport Mechanism

The OpenTofu Provider Protocol is designed with transport abstraction in mind, enabling future support for remote transports (HTTP, WebSocket, gRPC, etc.) while starting with stdio for simplicity and broad compatibility.

#### Standard I/O (stdio) Transport

**Initial Implementation Focus**: The stdio transport provides the foundation for provider communication:

**Stream Usage**:
- **stdin/stdout**: Protocol message communication using MessagePack-RPC
- **stderr**: Logging, debugging output, and diagnostic information

This separation follows the same pattern as [MCP servers](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports), providing authors with a clean way to emit logs and debugging information without interfering with protocol communication.

**Benefits of stdio Transport**:

**Simple Execution**: Providers can be executed as standard processes with no special networking requirements:
```bash
./my-provider --config=provider.json
```

**Container Compatibility**: Works seamlessly with containerized providers through Docker's stdio handling:
```bash
docker run --rm -i my-provider:latest
```

**Language Agnostic**: Every programming language has built-in support for reading from stdin and writing to stdout, making provider development accessible across ecosystems.

**Development Friendly**: Easy to test and debug providers using standard shell pipelines and tools.

#### Message Delimitation

All communication over stdio should use self-delimiting messages to ensure reliable parsing. This approach ensures that providers can be implemented without complex message framing logic, while maintaining compatibility with streaming parsers and network transports.

#### Future Transport Options

The protocol design enables future transport implementations:

**HTTP/HTTPS**: RESTful API endpoints for remote provider execution
**WebSocket**: Real-time bidirectional communication for streaming operations
**gRPC**: Integration with existing gRPC infrastructure
**Unix Domain Sockets**: Local high-performance communication
**TCP/TLS**: Direct network communication with encryption

**Transport Abstraction**: The RPC semantics remain identical across all transports - only the underlying message delivery mechanism changes. This allows providers written for stdio to be easily adapted for remote execution.

## Type Serialization

### Why MessagePack is Required

MessagePack is not just an optimization choice for OpenTofu providers - it is **technically required** to preserve the semantics of OpenTofu's type system, which is built on the [go-cty library](https://github.com/zclconf/go-cty).

**The Unknown Value Problem**: OpenTofu's two-phase execution model (plan-then-apply) requires "unknown values" (`cty.UnknownVal`) that represent placeholders for values not yet determinable during planning. These are essential when:
- Resource A depends on outputs from Resource B that doesn't exist yet
- Provider validation must work with incomplete information
- Change detection needs to distinguish known vs unknown future values

**JSON Cannot Represent OpenTofu's Type System**:
- **No unknown value support**: JSON has no way to represent `cty.UnknownVal`
- **Type information loss**: Lists/sets both become arrays, maps/objects both become objects
- **Precision loss**: Numbers lose integer vs float distinctions
- **Missing refined unknowns**: Cannot represent constraint metadata on unknown values

**Evidence from OpenTofu Core**: The existing provider protocol prefers MessagePack with JSON only as a compatibility fallback:
```go
// From internal/grpcwrap/provider6.go
switch {
case len(v.Msgpack) > 0:
    res, err = msgpack.Unmarshal(v.Msgpack, ty)  // Preferred
case len(v.Json) > 0:
    res, err = ctyjson.Unmarshal(v.Json, ty)     // Fallback only
}
```

### MessagePack Extensions for OpenTofu

OpenTofu uses specific MessagePack extension types to preserve type system fidelity, particularly for unknown values and their refinements. The most recent documentation of these extension codes and their payload formats can be found in the [Wire Format for OpenTofu Objects](../docs/plugin-protocol/object-wire-format.md) document.

### Performance Benefits

Beyond correctness, MessagePack provides performance advantages:
- **~30% smaller** than equivalent JSON representations
- **2-5x faster** serialization/deserialization
- **Streaming support**: Self-delimiting format enables efficient parsing
- **Binary efficiency**: Should hanndle large state files and complex configurations efficiently

## Core Message Types

Here are some examples of a possible set of messages that could be implemented to act as control messages before/after the opentofu flow occurs. Treat these as handshakes or ways to gracefully shut down.

### Initialization

**Init Request**: `["init", {"protocol_version": "1.0", "capabilities": [...]}]`
**Init Response**: `{"supported_capabilities": [...], "provider_info": {...}}`

Used for protocol negotiation and capability discovery.

### Provider Lifecycle

**Shutdown Notification**: `["shutdown", {}]`
Graceful termination request (no response expected).

**Ping Request**: `["ping", {}]`  
**Ping Response**: `{"status": "ok"}`
Health check and connectivity verification.

### Provider Operations

All standard `providers.Interface` methods map to protocol messages:

**GetProviderSchema**: Returns provider, resource, and data source schemas
**ValidateProviderConfig**: Validates provider configuration  
**ConfigureProvider**: Initializes provider with final configuration
**ReadResource**: Refreshes resource state
**PlanResourceChange**: Plans resource modifications
**ApplyResourceChange**: Applies planned changes

## Protocol Extensions

The protocol is designed to be extensible through additional message types and capabilities. Provider extension mechanisms will be detailed in a separate RFC document ([06-provider-extensions.md](./06-provider-extensions.md)).

**Capability Negotiation**: During initialization, providers return a list of supported capabilities in their init response. This allows OpenTofu Core to determine which extended features are available:

```
Init Response: {
  "supported_capabilities": ["middleware_hooks", "state_storage", "ai"],
  "provider_info": {...}
}
```

Future extensions may include middleware integration, provider state storage, cross-provider communication, and other advanced capabilities as the ecosystem evolves.