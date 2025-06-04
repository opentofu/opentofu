#!/usr/bin/env node

import {
  FileLogger,
  MiddlewareServer,
  StdioTransport,
  type PostPlanParams,
  type PrePlanParams,
} from "@opentofu/middleware";
import { estimateResourceCost, type CostEstimate } from "./estimator";

// Always log to stderr for debugging
console.error("[COST-ESTIMATOR] Starting cost estimator middleware...");

// Create logger if LOG_FILE env var is set
const logFile = process.env.COST_ESTIMATOR_LOG_FILE;
const logger = logFile ? new FileLogger(logFile) : undefined;

console.error(`[COST-ESTIMATOR] Log file: ${logFile || 'none'}`);

// Track costs across plan/apply lifecycle
const resourceCosts = new Map<string, CostEstimate>();

console.error("[COST-ESTIMATOR] Creating middleware server...");
// Create the middleware server
const server = new MiddlewareServer({
  name: "opentofu-cost-estimator",
  version: "0.1.0",
});
console.error("[COST-ESTIMATOR] Middleware server created");

// Log helper
function log(message: string): void {
  console.error(`[COST-ESTIMATOR] ${message}`);
  if (logger) {
    logger.log(message);
  }
}

console.error("[COST-ESTIMATOR] Setting up handlers...");

server
  .onInitialize(async (params: any) => {
    log(`Initialize called with: ${JSON.stringify(params)}`);
    return {
      capabilities: ["pre-plan", "post-plan", "post-apply"],
    };
  })
  .prePlan(async (params: PrePlanParams) => {
    const resourceId = `${params.resource_type}.${params.resource_name}`;
    log(`Pre-plan for ${resourceId}`);

    return {
      status: "pass",
      metadata: {
        middleware: "cost-estimator",
        timestamp: new Date().toISOString(),
        action: "pre-plan",
      },
    };
  })
  .postPlan(async (params: PostPlanParams) => {
    const resourceId = `${params.resource_type}.${params.resource_name}`;
    log(`Post-plan for ${resourceId} - Action: ${params.planned_action}`);

    // Only estimate costs for create/update actions
    if (params.planned_action === "Create" || params.planned_action === "Update") {
      const estimate = estimateResourceCost(
        params.resource_type,
        params.config,
        params.config?.tags,
      );

      if (estimate) {
        // Store the estimate for potential use in apply phase
        resourceCosts.set(resourceId, estimate);

        log(
          `Cost estimate for ${resourceId}: $${estimate.monthly}/month (${estimate.confidence} confidence)`,
        );

        return {
          status: "pass",
          message: `Estimated cost: $${estimate.monthly}/month`,
          metadata: {
            middleware: "cost-estimator",
            timestamp: new Date().toISOString(),
            action: "post-plan",
            resource: {
              type: params.resource_type,
              name: params.resource_name,
              provider: params.provider,
            },
            cost_estimate: estimate,
          },
        };
      }
    }

    return {
      status: "pass",
      metadata: {
        middleware: "cost-estimator",
        timestamp: new Date().toISOString(),
        action: "post-plan",
      },
    };
  })
  .postApply(async (params) => {
    const resourceId = `${params.resource_type}.${params.resource_name}`;
    const estimate = resourceCosts.get(resourceId);

    if (estimate && !params.failed) {
      log(`Resource ${resourceId} applied successfully. Cost: $${estimate.monthly}/month`);
    }

    return {
      status: "pass",
      metadata: {
        middleware: "cost-estimator",
        timestamp: new Date().toISOString(),
        action: "post-apply",
        cost_estimate: estimate,
      },
    };
  })
  .onShutdown(() => {
    log("Cost estimator shutting down");
  });

console.error("[COST-ESTIMATOR] Handlers set up");

console.error("[COST-ESTIMATOR] Creating transport...");
// Create transport and start
const transport = new StdioTransport({
  logger: (msg) => {
    console.error(`[TRANSPORT] ${msg}`);
    if (logger) {
      logger.log(`[TRANSPORT] ${msg}`);
    }
  },
});

console.error("[COST-ESTIMATOR] Connecting transport to server...");
// Start the middleware
transport.connect(server).then(() => {
  console.error("[COST-ESTIMATOR] Transport connected, middleware running");
}).catch((error) => {
  console.error(`[COST-ESTIMATOR] Failed to start: ${error}`);
  if (logger) {
    logger.log(`Failed to start: ${error}`);
  }
  process.exit(1);
});
