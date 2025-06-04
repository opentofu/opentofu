// OpenTofu Middleware TypeScript Type Definitions

export type ResourceMode = "managed" | "data";

export type PlanAction = "NoOp" | "Create" | "Update" | "Delete" | "Read";

export interface InitializeParams {
  version: string;
  name: string;
}

export interface InitializeResult {
  capabilities: MiddlewareCapability[];
}

export type MiddlewareCapability =
  | "pre-plan"
  | "post-plan"
  | "pre-apply"
  | "post-apply"
  | "pre-refresh"
  | "post-refresh";

// Previous middleware metadata passed through the chain
export type PreviousMiddlewareMetadata = Record<string, any>;

// Base params that include middleware chaining
interface BaseParams {
  provider: string;
  resource_type: string;
  resource_name: string;
  resource_mode: ResourceMode;
  previous_middleware_metadata?: PreviousMiddlewareMetadata;
}

export interface PrePlanParams extends BaseParams {
  config: any;
  current_state: any;
}

export interface PostPlanParams extends BaseParams {
  config: any;
  current_state: any;
  planned_state: any;
  planned_action: PlanAction;
}

export interface PreApplyParams extends BaseParams {
  config: any;
  current_state: any;
  planned_state: any;
  planned_action: PlanAction;
}

export interface PostApplyParams extends BaseParams {
  config: any;
  before: any;
  after: any;
  applied_action: PlanAction;
  failed: boolean;
}

export interface PreRefreshParams extends BaseParams {
  current_state: any;
}

export interface PostRefreshParams extends BaseParams {
  before: any;
  after: any;
  drift_detected: boolean;
}

export type HookStatus = "pass" | "fail" | "error";

export interface HookResult {
  status: HookStatus;
  message?: string;
  metadata?: any;
}

// JSON-RPC types
export interface JsonRpcRequest<T = any> {
  jsonrpc: "2.0";
  method: string;
  params?: T;
  id: number | string;
}

export interface JsonRpcResponse<T = any> {
  jsonrpc: "2.0";
  result?: T;
  error?: JsonRpcError;
  id: number | string;
}

export interface JsonRpcError {
  code: number;
  message: string;
  data?: any;
}

// Standard JSON-RPC error codes
export const JsonRpcErrorCodes = {
  PARSE_ERROR: -32700,
  INVALID_REQUEST: -32600,
  METHOD_NOT_FOUND: -32601,
  INVALID_PARAMS: -32602,
  INTERNAL_ERROR: -32603,
} as const;

// Method type mapping
export type MethodParams = {
  initialize: InitializeParams;
  "pre-plan": PrePlanParams;
  "post-plan": PostPlanParams;
  "pre-apply": PreApplyParams;
  "post-apply": PostApplyParams;
  "pre-refresh": PreRefreshParams;
  "post-refresh": PostRefreshParams;
  ping: object;
  shutdown: null;
};

export type MethodResult = {
  initialize: InitializeResult;
  "pre-plan": HookResult;
  "post-plan": HookResult;
  "pre-apply": HookResult;
  "post-apply": HookResult;
  "pre-refresh": HookResult;
  "post-refresh": HookResult;
  ping: { message: string };
  shutdown: string;
};
