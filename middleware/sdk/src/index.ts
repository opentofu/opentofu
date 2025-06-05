// Main entry point for the OpenTofu middleware library

export * from "./types";
export * from "./middleware";
export * from "./server";
export * from "./transport";
export * from "./stdio-transport";
export * from "./utils/chain";
export * from "./utils/resource";
export * from "./utils/file-logger";
export * from "./utils/plan";

// Re-export commonly used types for convenience
export type {
  HookResult,
  HookStatus,
  MiddlewareCapability,
  PreviousMiddlewareMetadata,
  PrePlanParams,
  PostPlanParams,
  PreApplyParams,
  PostApplyParams,
  PreRefreshParams,
  PostRefreshParams,
  OnPlanCompletedParams,
  Plan,
  ResourceChange,
  Change,
  Variables,
  Variable,
  StateValues,
  Output,
  Module,
  Resource,
  AttributeValues,
  Importing,
  ResourceAttr,
} from "./types";

export type { PlanSummary, ResourceComparison } from "./utils/plan";

export {
  Middleware,
  type MiddlewareHandlers,
  type MiddlewareOptions,
} from "./middleware";

export {
  MiddlewareServer,
  type MiddlewareServerOptions,
} from "./server";

export type { Transport } from "./transport";

export {
  StdioTransport,
  type StdioTransportOptions,
} from "./stdio-transport";
