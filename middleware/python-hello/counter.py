#!/usr/bin/env python3
import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'python-sdk', 'src'))

from opentofu_middleware import (
    HookResult,
    MiddlewareServer,
    OnPlanCompletedParams,
    PostPlanParams,
    StdioTransport,
)

# Counter to track resources
resource_count = 0

# Create server
server = MiddlewareServer("resource-counter", "1.0.0")

# Post-plan hook - count resources
@server.post_plan
def post_plan(params: PostPlanParams) -> HookResult:
    global resource_count
    resource_count += 1
    return HookResult(
        status="pass",
        metadata={"count": resource_count}
    )

# Plan completed hook - report total
@server.on_plan_completed
def on_plan_completed(params: OnPlanCompletedParams) -> HookResult:
    global resource_count
    return HookResult(
        status="pass",
        message=f"Total resources: {resource_count}",
        metadata={"total_resources": resource_count}
    )

# Start the server
if __name__ == "__main__":
    transport = StdioTransport()
    transport.connect(server)
    transport.start()