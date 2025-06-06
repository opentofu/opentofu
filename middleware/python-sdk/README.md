# OpenTofu Middleware Python SDK

A Python SDK for building OpenTofu middleware that intercepts and augments provider operations.

## Installation

```bash
pip install opentofu-middleware
```

Or install from source:
```bash
cd middleware/python-sdk
pip install -e .
```

## Quick Start

Create a simple middleware in just a few lines:

```python
#!/usr/bin/env python3
from opentofu_middleware import HookResult, MiddlewareServer, PostPlanParams, StdioTransport

# Create server
server = MiddlewareServer("my-middleware", "1.0.0")

# Add a post-plan handler
@server.post_plan
def handle_post_plan(params: PostPlanParams) -> HookResult:
    return HookResult(
        status="pass",
        message=f"Processed {params.resource_type}.{params.resource_name}",
        metadata={"processed": True}
    )

# Start listening
if __name__ == "__main__":
    transport = StdioTransport()
    transport.connect(server)
    transport.start()
```

## Complete Example

Here's a more comprehensive example showing all available hooks:

```python
from opentofu_middleware import (
    HookResult,
    MiddlewareServer,
    OnPlanCompletedParams,
    PostApplyParams,
    PostPlanParams,
    PostRefreshParams,
    PreApplyParams,
    PrePlanParams,
    PreRefreshParams,
    StdioTransport,
)

# Create server with logging
server = MiddlewareServer("advanced-middleware", "1.0.0")

# Pre-plan hook - can modify configuration
@server.pre_plan
def pre_plan(params: PrePlanParams) -> HookResult:
    # Add tags to all resources
    config = params.config or {}
    if "tags" not in config:
        config["tags"] = {}
    config["tags"]["ManagedBy"] = "OpenTofu"
    
    return HookResult(
        status="pass",
        modified_config=config,
        metadata={"tags_added": True}
    )

# Post-plan hook - review planned changes
@server.post_plan
def post_plan(params: PostPlanParams) -> HookResult:
    if params.planned_action == "Delete":
        return HookResult(
            status="fail",
            message="Deletion not allowed by policy"
        )
    
    return HookResult(
        status="pass",
        metadata={"reviewed": True}
    )

# Pre-apply hook - final approval
@server.pre_apply
def pre_apply(params: PreApplyParams) -> HookResult:
    # Could check external approval system here
    return HookResult(
        status="pass",
        message="Approved for deployment"
    )

# Post-apply hook - track deployed resources
@server.post_apply
def post_apply(params: PostApplyParams) -> HookResult:
    # Metadata from this hook is persisted in state
    return HookResult(
        status="pass",
        metadata={
            "deployed_at": datetime.utcnow().isoformat(),
            "deployed_by": "middleware",
            "resource_id": params.after.get("id") if params.after else None,
        }
    )

# Plan completed hook - receives full plan JSON
@server.on_plan_completed
def on_plan_completed(params: OnPlanCompletedParams) -> HookResult:
    plan = params.plan_json
    resource_count = len(plan.get("resource_changes", []))
    
    if resource_count > 100:
        return HookResult(
            status="fail",
            message="Plan too large: maximum 100 resources allowed"
        )
    
    return HookResult(
        status="pass",
        message=f"Plan validated: {resource_count} resources",
        metadata={"resource_count": resource_count}
    )

# Start the server
if __name__ == "__main__":
    transport = StdioTransport()
    transport.connect(server)
    transport.start()
```

## Using in OpenTofu

Configure your middleware in the OpenTofu configuration:

```hcl
middleware "cost_estimator" {
  command = "python3"
  args    = ["/path/to/your/middleware.py"]
  env = {
    LOG_LEVEL = "DEBUG"
  }
}

provider "aws" {
  region = "us-east-1"
  middleware = [middleware.cost_estimator]
}
```

## Available Hooks

### Resource-Level Hooks
- **pre_plan**: Called before planning each resource (can modify config)
- **post_plan**: Called after planning each resource
- **pre_apply**: Called before applying changes to each resource
- **post_apply**: Called after applying changes (metadata persisted to state)
- **pre_refresh**: Called before refreshing each resource
- **post_refresh**: Called after refreshing each resource

### Plan-Level Hooks
- **on_plan_completed**: Called with full plan JSON after planning completes

## Hook Results

All hooks return a `HookResult` with:
- `status`: "pass" or "fail"
- `message`: Optional human-readable message
- `metadata`: Optional dictionary (persisted for post-apply hooks)
- `modified_config`: Config modifications (pre-plan only)

## Utilities

The SDK includes helpful utilities:

```python
from opentofu_middleware.utils import CostEstimator, FileLogger, ResourceUtils

# File logging
logger = FileLogger("/tmp/middleware.log")
logger.info("Processing resource")

# Resource utilities
address = ResourceUtils.get_resource_address("aws_instance", "web")
provider_parts = ResourceUtils.parse_provider("hashicorp/aws")
tags = ResourceUtils.extract_tags(config)

# Cost estimation
cost = CostEstimator.estimate_resource_cost("aws_instance", config, tags)
```

## Development

### Running Tests
```bash
pip install -e ".[dev]"
pytest
```

### Type Checking
```bash
mypy src/opentofu_middleware
```

### Code Formatting
```bash
black src/opentofu_middleware
```

## Advanced Features

### Custom Transport

While the SDK provides `StdioTransport` for JSON-RPC over stdin/stdout, you can implement custom transports:

```python
from opentofu_middleware import Transport

class HttpTransport(Transport):
    def connect(self, server: MiddlewareServer) -> None:
        # Implementation
        pass
    
    def start(self) -> None:
        # Start HTTP server
        pass
    
    def stop(self) -> None:
        # Stop HTTP server
        pass
```

### Error Handling

The SDK handles errors gracefully:

```python
@server.post_plan
def post_plan(params: PostPlanParams) -> HookResult:
    try:
        # Your logic here
        result = expensive_calculation()
        return HookResult(status="pass", metadata={"result": result})
    except Exception as e:
        # Log error (to stderr or file, not stdout)
        logger.error(f"Calculation failed: {e}")
        # Return graceful failure
        return HookResult(
            status="pass",  # Don't fail the plan
            message="Cost estimation unavailable",
            metadata={"error": str(e)}
        )
```

## License

MPL-2.0