# OpenTofu Middleware For Enhanced Operations

### Introduction
Today, a rich ecosystem of third-party tools has emerged around Terraform and OpenTofu that analyze plan files, parse HCL configurations, and enforce policies. Tools like OPA (Open Policy Agent), Sentinel, Checkov, Infracost, and others have proven invaluable for organizations implementing governance, compliance, and cost controls. However, these tools operate entirely outside the OpenTofu execution flow, requiring users to integrate them through CI/CD pipelines, wrapper scripts, or manual processes. This separation creates several challenges:

- **Timing gaps**: Policies are evaluated after plans are generated, missing opportunities for early validation
- **Limited context**: External tools only see what's exported to plan files, missing runtime state and provider-specific details  
- **Complex workflows**: Users must orchestrate multiple tools and handle failures across disconnected systems
- **No feedback loop**: External tools cannot influence OpenTofu's behavior or provide metadata back to the state

This RFC proposes bringing this extensibility directly into OpenTofu through a middleware system that allows users to intercept operations at key points during execution. By providing official hook points and a read-only, well-defined protocol, we can enable the same policy enforcement, cost estimation, and compliance validation use cases while offering several advantages:

- **Real-time validation**: Catch issues during planning, not after
- **Rich context**: Access to full resource state, provider details, and operation context
- **Simplified workflows**: Middleware runs as part of normal OpenTofu operations
- **Bidirectional communication**: Middleware can fail operations and store metadata in state
- **Language agnostic**: Write middleware in any language

By offering this extensibility through a well-defined protocol, we aim to enable organizations to bring their existing policy and governance tools into the OpenTofu execution flow while fostering innovation in new use cases that weren't possible with external analysis alone.

The architecture of this middleware system is inspired by the recent Model Context Protocol (MCP) approach, which has proven successful for extending AI assistants through local server processes communicating via JSON-RPC over stdio (along with other transports). This design pattern offers simplicity, language independence, and process isolation - qualities that align perfectly with OpenTofu's needs for a stable, extensible middleware system without the overhead of producing a full SDK or requiring middleware authors to be locked into a specific languge/ecosystem. You can find more information about this specification [at modelcontextprotocol.io/docs](https://modelcontextprotocol.io/docs/concepts/architecture).

## Approach
To provide users with a flexible and powerful way to extend OpenTofu's functionality, I propose implementing a middleware system that communicates with external processes via JSON-RPC 2.0 over STDIO. The middleware will be invoked at specific hook points during OpenTofu operations, allowing users to inspect, validate, and augment the behavior of resource operations without requiring changes to OpenTofu's core. 

When it comes to augmentation of the resource operations, I propose that initially the middleware cannot alter the state or inputs to a resource/datasource etc. I believe that providing a read only system that can return failures and metadata is the safest way to go and then there is less risk for the end users too. Less "magic" altering things means that the config is the source of truth.

### Hook Points for Operations
The middleware system will provide hook points at two levels:

**Resource-level hooks** (called for each individual resource/data source):
- **pre-plan**
- **post-plan**
- **pre-apply**
- **post-apply**
- **pre-refresh**
- **post-refresh**

**Operation-level hooks** (called once per operation):
- **init-stage-start**
- **init-stage-complete**
- **plan-stage-start**
- **plan-stage-complete**
- **apply-stage-start**
- **apply-stage-complete**

Additional hooks can be added in future versions as use cases emerge. It is possible that we should also consider hooks that handle failures/errors too.

### Middleware Attachments

For now I propose that middleware should be "attachable" to the entire project, or to a specific provider instance. This opens up the prospect of having provider-extension middleware or project-wide enforcement of rules.

### Configuration Design
Users will configure middleware through HCL blocks in their OpenTofu configuration:

```hcl

middleware "naming_convention_checker" {
  metadata_key = "naming"
  command = "lint-opentofu-names"
}

middleware "cost_estimator" {
  metadata_key = "cost"
  command = "python3"
  args    = ["cost_estimator.py"]
  env = {
    API_KEY = var.cost_api_key
  }
}

terraform { 
  middleware = [middleware.naming_convention_checker] # Attach it across the entire project
}

provider "aws" {
  middleware = [middleware.cost_estimator] # Or just attach it to a single provider
  region = "us-east-1"
}
```

### Middleware Execution Order

When multiple middleware are attached to a provider or project, they execute in the order defined in the middleware array. This deterministic ordering allows for:

- **Pipeline Processing**: Earlier middleware can validate/transform data for later middleware
- **Priority Control**: Critical security checks can run before cost estimation
- **Dependency Management**: Middleware that depends on others' metadata can run later

Example execution flow:
```hcl
provider "aws" {
  middleware = [
    middleware.security_scanner,    # Runs first - can fail fast on violations
    middleware.compliance_checker,  # Runs second - sees security metadata
    middleware.cost_estimator       # Runs last - only if security/compliance pass
  ]
}
```

For pre-hooks (pre-plan, pre-apply, etc.), if any middleware returns a "fail" status, execution stops immediately and subsequent middleware in the chain are not called. For post-hooks, all middleware execute regardless of individual failures, allowing comprehensive metadata collection.

### Execution Hierarchy

When middleware is attached at both the project level (via `terraform` block) and provider level, the execution order follows a clear hierarchy:

1. **Project-level middleware executes first** - Applied to all resources regardless of provider
2. **Provider-level middleware executes second** - Applied only to resources from that provider

This hierarchy allows for:
- Global policies to be enforced before provider-specific checks
- Provider middleware to see and build upon project middleware metadata
- Layered validation from general to specific

#### Duplicate Middleware Handling

If the same middleware is attached at both levels, it will execute **twice** for resources from that provider:

```hcl
middleware "cost_estimator" {
  middleware_key = "cost"
  command = "cost-estimate"
}

terraform {
  middleware = [middleware.cost_estimator]  # First execution
}

provider "aws" {
  middleware = [middleware.cost_estimator]  # Second execution - overwrites metadata!
}
```

In this case:
- The middleware runs twice for AWS resources
- The second execution's metadata **overwrites** the first
- Each execution has no knowledge it's being run multiple times

Users should generally avoid this pattern unless the middleware is designed to handle multiple executions.

### Communication
As mentioned above, I propose that to keep it as open as possible we should use JSON-RPC 2.0 over stdio for communication with the possibility to open up to other transports in the future.

#### Initialize Handshake
```json
// Request
{
  "jsonrpc": "2.0",
  "method": "initialize",
  "params": {
    "version": "1.0",
    "name": "cost_estimator"
  },
  "id": 1
}

// Response
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "capabilities": ["pre-plan", "post-plan", "pre-apply", "post-apply"]
  }
}
```

#### Hook Execution
```json
// Post-Plan Hook Request
{
  "jsonrpc": "2.0",
  "method": "post-plan",
  "params": {
    "provider": "registry.opentofu.org/hashicorp/aws",
    "resource_type": "aws_instance",
    "resource_name": "example",
    "resource_mode": "managed",
    "planned_action": "Create",
    "before": null,
    "after": { /* planned state */ },
    "config": { /* resource config */ }
    ....
  },
  "id": 2
}

// Hook Response
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "status": "success",
    "message": "Estimated monthly cost: $73.58",
    "metadata": {
      "estimated_cost": {
        "hourly": 0.102,
        "monthly": 73.58,
        "currency": "USD"
      }
    }
  }
}
```

### Process Lifecycle

Middleware processes are managed with a simple lifecycle:

1. **Startup**: When OpenTofu begins execution (init, plan, apply, etc.), all configured middleware processes are started
2. **Initialization**: Each middleware receives an initialize request and must respond with its capabilities
3. **Operation**: Middleware processes remain running for the entire duration of the OpenTofu command
4. **Shutdown**: When OpenTofu completes (successfully or with errors), all middleware processes are terminated

This approach ensures:
- Middleware can maintain state across multiple hook invocations
- Startup overhead is incurred only once per OpenTofu run
- Clean shutdown prevents orphaned processes

### Failure Handling

Middleware can cause OpenTofu operations to fail by returning a "fail" status with an error message:

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "status": "fail",
    "message": "S3 bucket 'prod-data' is not encrypted. All production buckets must have encryption enabled per security policy SEC-2023-001."
  }
}
```

When middleware returns a failure:
- **All hooks**: The OpenTofu operation is aborted with an error
- **Custom error messages**: The middleware's message is displayed to the user
- **Exit codes**: OpenTofu exits with a non-zero code, failing CI/CD pipelines
- **State consistency**: For post-apply failures, the state is still saved but the operation fails

This enables middleware to enforce hard policies and provide clear, actionable error messages to users.

### Key Benefits of This Approach

- **Language Agnostic**: Middleware can be written in any language that supports stdio
- **Process Isolation**: Middleware runs in separate processes, ensuring stability
- **Flexible Protocol**: JSON-RPC provides versioning and extensibility
- **Metadata Persistence**: Post-apply metadata is stored in state for auditing

### Metadata Storage and Persistence
Middleware-returned metadata should be attached to the target in OpenTofu's state and plan files. This means that other processes or middleware should be able to examine the output of other middleware. Making them chainable.

Each middleware requires a "metadata_key" field to be populated in the middleware block. This key allows the properties deterministic and allows for control over how chainable middleware can handle referencing other's metadata output.

```json
{
  "__middleware_metadata__": {
    "cost_estimator": {
      "middleware": "cost-estimator",
      "action": "post-apply",
      "resource": {
        "type": "aws_instance",
        "name": "example",
        "provider": "registry.opentofu.org/hashicorp/aws"
      },
      "applied_action": "Create",
      "actual_cost": {
        "billing_period": "hourly",
        "cost_incurred": 0,
        "currency": "USD"
      },
      "timestamp": "2025-07-11T10:30:00Z"
    }
  }
}
```

## Implementation Details and Architecture

### Core Components

#### 1. Middleware Manager
The middleware manager orchestrates all middleware operations:
- Process lifecycle management (start/stop)
- Hook invocation and response aggregation
- Timeout and error handling
- Metadata collection and namespacing

#### 2. JSON-RPC Client
Handles communication with middleware processes:
- Request/response serialization
- Error handling and retries
- Protocol version negotiation

#### 3. Configuration Integration
Extends HCL parsing to support middleware blocks:
- Middleware block validation
- Provider middleware references
- Environment variable handling

## Benefits

### For End Users
- **Policy Enforcement**: Implement organization-specific compliance rules
- **Cost Control**: Estimate and track infrastructure costs in real-time
- **Custom Integrations**: Connect OpenTofu to internal systems and workflows
- **Enhanced Visibility**: Add custom logging and metrics collection

### For the Ecosystem
- **Extensibility Without Forking**: Organizations can extend OpenTofu without maintaining forks
- **Community Middleware**: Shareable middleware for common use cases
- **Provider Ecosystem**: Middleware can complement provider functionality

## SDK Support

While the JSON-RPC protocol is simple enough to implement directly, the OpenTofu team could provide official SDKs to accelerate middleware development. For example, a TypeScript SDK could make building middleware as simple as:

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

SDKs would handle protocol communication, type safety, and error handling, letting developers focus on their middleware logic.

## Open Questions

### Performance and Scalability
- What is the performance impact of middleware on large infrastructures?
- Should middleware calls be parallelized for independent resources?
- How do we handle middleware that needs to see all resources before making decisions?

### Security Considerations
- Should we sanitize the input to the middleware to hide sensitive fields?
- Should there be restrictions on what middleware can access?

### Protocol Evolution
- How do we version the middleware protocol for future changes?
- Should we support multiple protocol versions simultaneously?
- How do we handle breaking changes in the protocol?

## Current Proof of Concept Implementation Status

### Completed
- ✅ Core middleware system with most hooks
- ✅ JSON-RPC protocol implementation
- ✅ Process management and lifecycle
- ✅ Metadata storage in state (post-apply only, post-plan still needed)
- ✅ TypeScript SDK for middleware development
- ✅ Python SDK for middleware development
- ✅ Multiple example middlewares
- ✅ Integration with OpenTofu's config layer

### Known Limitations
- ❌ No cross-resource visibility (each hook sees one resource at a time, or everything all at once, nothing inbetween)
- ❌ Limited to stdio communication (no network transports)

## Conclusion

The OpenTofu middleware system provides a powerful, flexible way for organizations to extend and customize OpenTofu's behavior without modifying its core. By offering well-defined hook points and a simple communication protocol, we enable a wide range of use cases from cost management to compliance enforcement.

The current implementation provides a solid foundation with room for future enhancements based on community needs. The architecture's simplicity and language-agnostic design ensure that middleware development is accessible to a broad audience while maintaining the stability and reliability that OpenTofu users expect.

This middleware system represents a significant step toward making OpenTofu more adaptable to diverse organizational requirements while fostering a ecosystem of reusable extensions.