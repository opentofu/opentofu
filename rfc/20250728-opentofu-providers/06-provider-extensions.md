# Provider Extensions

## Summary

This document outlines the extensibility philosophy for the OpenTofu provider protocol and introduces two major potential extensions: middleware integration and state management enhancements. The goal is to design an evolving protocol where new features can be added over time without breaking backward compatibility.

### HTTP-like Protocol Evolution

Similar to how HTTP has evolved over time (HTTP/1.0 → HTTP/1.1 → HTTP/2 → HTTP/3) while maintaining backward compatibility, the OpenTofu provider protocol should be designed to grow incrementally. Old versions of OpenTofu should be able to communicate with newer providers by simply using the subset of functionality they understand. This approach was inspired by @apparentlymart's insights on protocol evolution patterns.

### Core Compatibility Principle

The fundamental principle is that **any OpenTofu version should be able to talk to any provider version**. If OpenTofu doesn't understand middleware hooks, it simply doesn't use them. If a provider doesn't support batching, operations happen one at a time. The core CRUD operations remain universal and always work.

## Proposed Extensions

We are proposing two major extensions to demonstrate how the protocol can evolve:

### 1. Middleware Integration

Middleware allows interception and modification of provider operations, enabling powerful features like cost tracking, approval gates, and policy enforcement without requiring changes to individual providers.

**See**: [06a-middleware.md](./06a-middleware.md) for detailed specification.

### 2. State Management Enhancements  

Enhanced state management capabilities including local caching, state transformation, and improved dependency tracking to optimize performance and provide new capabilities.

**See**: [06b-state-management.md](./06b-state-management.md) for detailed specification.

## Protocol Evolution Strategy

### Graceful Degradation

The protocol should be designed so that:

1. **Old OpenTofu + New Provider**: Works perfectly using basic functionality
2. **New OpenTofu + Old Provider**: Works perfectly, advanced features simply aren't available
3. **New OpenTofu + New Provider**: Can negotiate and use advanced features

### Feature Detection

Feature detection uses the capabilities system defined in the [provider protocol document](./01-provider-protocol.md). During the initialization handshake, providers declare their supported capabilities:

**Init Request** from OpenTofu:
```json
["init"]
```

**Init Response** from Provider:
```json
{
  "supported_capabilities": [
    "managed_resources",
    "functions", 
    "middleware_hooks", 
    "state_caching"
  ],
  "provider_info": {...}
}
```

This allows:
- **Providers** to declare which capabilities they implement  
- **OpenTofu** to use only the capabilities it understands
- **Graceful degradation** when OpenTofu doesn't understand a capability

## Implementation Considerations

### Incremental Development

Rather than building a complex extension system upfront, features could be added incrementally over time.

### Backward Compatibility Testing

Any protocol changes should be tested to ensure:
- Old OpenTofu versions can still use new providers
- New OpenTofu versions gracefully handle old providers
- Core functionality is never compromised

### Provider Choice

Providers should have complete freedom to choose which enhancements to implement. A simple provider might only implement basic CRUD, while an enterprise provider might implement the full feature set. It's also possible for a provider to just handle functions.

## Examples of Future Features

### Middleware Configuration

```hcl
terraform {
  required_providers {
    myapp = {
      cmd  = "python3"
      args = ["./myapp-provider.py"]
      
      # Only used if both OpenTofu and provider support middleware
      middleware = {
        cost_tracking = {
          budget_limit = 1000
        }
        approval_gate = {
          require_approval = true
        }
      }
    }
  }
}
```

### Batch Operation Support

```json
{
  "batch_request": {
    "operations": [
      {"type": "create", "resource": "user1", "config": {...}},
      {"type": "create", "resource": "user2", "config": {...}},
      {"type": "update", "resource": "user3", "config": {...}}
    ]
  }
}
```

### State Caching Headers

```json
{
  "read_response": {
    "state": {...},
    "cache_ttl": 300,
    "etag": "abc123"
  }
}
```

## Conclusion

The key is to design these features as optional enhancements that gracefully degrade when not supported, rather than as required capabilities that create compatibility matrices.