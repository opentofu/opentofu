#!/usr/bin/env python3
import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'python-sdk', 'src'))

from opentofu_middleware import (
    HookResult,
    MiddlewareServer,
    PostPlanParams,
    StdioTransport,
)

# Create server
server = MiddlewareServer("hello-middleware", "1.0.0")

# Post-plan hook - greet each resource
@server.post_plan
def post_plan(params: PostPlanParams) -> HookResult:
    resource = f"{params.resource_type}.{params.resource_name}"
    return HookResult(
        status="pass",
        message=f"Hello from Python! I see {resource}",
        metadata={"greeted": resource}
    )

# Start the server
if __name__ == "__main__":
    transport = StdioTransport()
    transport.connect(server)
    transport.start()