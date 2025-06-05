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
  | "post-refresh"
  | "on-plan-completed";

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

// Plan-level hook parameters
export interface OnPlanCompletedParams {
  plan_json: Plan; // Full plan JSON with strong typing
  success: boolean;
  errors?: string[];
  previous_middleware_metadata?: PreviousMiddlewareMetadata;
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
  "on-plan-completed": OnPlanCompletedParams;
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
  "on-plan-completed": HookResult;
  ping: { message: string };
  shutdown: string;
};

// ===================================================================
// COMPREHENSIVE JSON PLAN TYPES (based on internal/command/jsonplan)
// ===================================================================

// Top-level plan structure
export interface Plan {
  format_version?: string;
  terraform_version?: string;
  variables?: Variables;
  planned_values?: StateValues;
  resource_drift?: ResourceChange[];
  resource_changes?: ResourceChange[];
  output_changes?: Record<string, Change>;
  prior_state?: StateValues; // Now properly typed with the same structure as planned_values
  configuration?: any; // JSON object
  relevant_attributes?: ResourceAttr[];
  checks?: any; // JSON object
  timestamp?: string;
  errored: boolean;
}

// Variables types
export type Variables = Record<string, Variable>;

export interface Variable {
  value?: any; // JSON value
}

// State values structure
export interface StateValues {
  outputs?: Record<string, Output>;
  root_module?: Module;
}

// Output structure
export interface Output {
  sensitive: boolean;
  deprecated?: string;
  type?: any; // JSON type representation
  value?: any; // JSON value
}

// Module structure
export interface Module {
  resources?: Resource[];
  address?: string; // omitted for root module
  child_modules?: Module[];
}

// Resource structure (for planned values)
export interface Resource {
  address?: string;
  mode?: string; // "managed" | "data"
  type?: string;
  name?: string;
  index?: any; // JSON representation of instance key
  provider_name?: string;
  schema_version: number;
  values?: AttributeValues;
  sensitive_values?: any; // JSON object
}

// Attribute values
export type AttributeValues = Record<string, any>;

// Resource change structure
export interface ResourceChange {
  address?: string;
  previous_address?: string;
  module_address?: string;
  mode?: string; // "managed" | "data"
  type?: string;
  name?: string;
  index?: any; // JSON representation
  provider_name?: string;
  deposed?: string;
  change?: Change;
  action_reason?: string;
}

// Change structure
export interface Change {
  actions?: string[]; // ["no-op"] | ["create"] | ["read"] | ["update"] | ["delete", "create"] | ["create", "delete"] | ["delete"] | ["forget"]
  before?: any; // JSON value
  after?: any; // JSON value
  after_unknown?: any; // JSON object
  before_sensitive?: any; // JSON object  
  after_sensitive?: any; // JSON object
  replace_paths?: any; // JSON array of arrays
  importing?: Importing;
  generated_config?: string;
}

// Importing structure
export interface Importing {
  id?: string;
}

// Resource attribute for relevant attributes
export interface ResourceAttr {
  resource: string;
  attribute: any; // JSON representation of attribute path
}
