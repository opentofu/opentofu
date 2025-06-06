#!/usr/bin/env python3
import sys
import os
import argparse
import json
import tempfile
import logging
from typing import Dict, Any
from pathlib import Path

# Force unbuffered output
sys.stdout = os.fdopen(sys.stdout.fileno(), 'w', 1)  # Line buffered
sys.stderr = os.fdopen(sys.stderr.fileno(), 'w', 1)  # Line buffered

# Add the python-sdk to the path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'middleware', 'python-sdk', 'src'))

from opentofu_middleware import (
    HookResult,
    MiddlewareServer,
    OnPlanCompletedParams,
    PostPlanParams,
    StdioTransport,
)

# Set up logging to stderr
logging.basicConfig(
    level=logging.INFO,
    format='[%(levelname)s] %(message)s',
    handlers=[logging.StreamHandler(sys.stderr)]
)
logger = logging.getLogger(__name__)

# Parse command line arguments
parser = argparse.ArgumentParser(description='Cost Manager Middleware - Enforces budget limits')
parser.add_argument('--max-budget', type=float, default=100.0, 
                    help='Maximum monthly budget in dollars (default: $100)')
args = parser.parse_args()

logger.info(f"Cost Manager started with max budget: ${args.max_budget}")

# Track costs from other middleware
resource_costs: Dict[str, Dict[str, Any]] = {}
total_monthly_cost = 0.0

# Create server
server = MiddlewareServer("cost-manager", "1.0.0")

# Post-plan hook - extract cost information from previous middleware
@server.post_plan
def post_plan(params: PostPlanParams) -> HookResult:
    global resource_costs, total_monthly_cost
    
    resource_id = f"{params.resource_type}.{params.resource_name}"
    logger.info(f"Processing resource: {resource_id} (action: {params.planned_action})")
    found_cost_data = False
    
    # Look for cost information from cost_estimator middleware
    if params.previous_middleware_metadata:
        logger.info(f"Resource {resource_id}: Found middleware metadata from: {list(params.previous_middleware_metadata.keys())}")
        
        for middleware_name, metadata in params.previous_middleware_metadata.items():
            if 'cost_estimator' in middleware_name and isinstance(metadata, dict):
                logger.debug(f"  Found cost_estimator metadata: {list(metadata.keys()) if metadata else 'None'}")
                if 'cost_estimate' in metadata:
                    cost_info = metadata['cost_estimate']
                    # Store the cost information
                    resource_costs[resource_id] = {
                        'action': params.planned_action,
                        'monthly_cost': cost_info.get('monthly', 0),
                        'currency': cost_info.get('currency', 'USD'),
                        'resource_type': params.resource_type,
                        'resource_name': params.resource_name
                    }
                    if params.planned_action in ['Create', 'Update']:
                        total_monthly_cost += cost_info.get('monthly', 0)
                    
                    logger.info(f"Resource {resource_id}: ${cost_info.get('monthly', 0)}/month ({params.planned_action})")
                    found_cost_data = True
                    break
                else:
                    logger.warning(f"Resource {resource_id}: cost_estimator metadata found but no 'cost_estimate' field. Available fields: {list(metadata.keys())}")
    else:
        logger.warning(f"Resource {resource_id}: No previous middleware metadata found")
    
    if not found_cost_data:
        logger.warning(f"Resource {resource_id}: No cost data found from any middleware")
    
    # Always pass individual resources
    return HookResult(
        status="pass",
        metadata={
            "resource": resource_id,
            "tracked_cost": resource_costs.get(resource_id, {}).get('monthly_cost', 0),
            "has_cost_data": found_cost_data
        }
    )

# Plan completed hook - check total cost against budget
@server.on_plan_completed
def on_plan_completed(params: OnPlanCompletedParams) -> HookResult:
    global total_monthly_cost, resource_costs
    
    logger.info(f"Plan completed. Processed {len(resource_costs)} resources")
    logger.info(f"Total monthly cost: ${total_monthly_cost:.2f}")
    logger.info(f"Budget limit: ${args.max_budget:.2f}")
    
    # Check if we're over budget
    if total_monthly_cost > args.max_budget:
        # Generate detailed cost breakdown
        cost_breakdown = []
        for resource_id, cost_info in sorted(resource_costs.items(), 
                                           key=lambda x: x[1]['monthly_cost'], 
                                           reverse=True):
            if cost_info['action'] in ['Create', 'Update']:
                cost_breakdown.append(
                    f"  - {resource_id}: ${cost_info['monthly_cost']:.2f}/month"
                )
        
        breakdown_text = "\n".join(cost_breakdown[:10])  # Show top 10 resources
        if len(cost_breakdown) > 10:
            breakdown_text += f"\n  ... and {len(cost_breakdown) - 10} more resources"
        
        error_message = (
            f"❌ BUDGET EXCEEDED: Plan would cost ${total_monthly_cost:.2f}/month, "
            f"which exceeds the budget limit of ${args.max_budget:.2f}/month.\n\n"
            f"Top resource costs:\n{breakdown_text}\n\n"
            f"To proceed, either:\n"
            f"1. Reduce the number or size of resources\n"
            f"2. Increase the budget limit with --max-budget\n"
            f"3. Remove the cost-manager middleware"
        )
        
        logger.error(f"Budget exceeded! ${total_monthly_cost:.2f} > ${args.max_budget:.2f}")
        
        return HookResult(
            status="fail",
            message=error_message,
            metadata={
                "total_monthly_cost": total_monthly_cost,
                "budget_limit": args.max_budget,
                "exceeded_by": total_monthly_cost - args.max_budget,
                "resource_count": len(resource_costs),
                "resources": resource_costs
            }
        )
    else:
        success_message = (
            f"✅ Budget check passed: ${total_monthly_cost:.2f}/month "
            f"(within ${args.max_budget:.2f} limit)"
        )
        
        logger.info("Budget check passed")
        
        return HookResult(
            status="pass",
            message=success_message,
            metadata={
                "total_monthly_cost": total_monthly_cost,
                "budget_limit": args.max_budget,
                "remaining_budget": args.max_budget - total_monthly_cost,
                "resource_count": len(resource_costs)
            }
        )

# Start the server
if __name__ == "__main__":
    transport = StdioTransport()
    transport.connect(server)
    logger.info("Cost Manager middleware ready")
    transport.start()