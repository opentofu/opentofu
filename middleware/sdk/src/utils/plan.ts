import type { Change, Plan, ResourceChange, Output, Resource, Module, Variables } from "../types";

/**
 * Helper utilities for working with OpenTofu plan files
 */

/**
 * Filter resources by action type
 */
export function getResourcesByAction(plan: Plan, ...actions: string[]): ResourceChange[] {
  if (!plan.resource_changes) return [];

  return plan.resource_changes.filter((resource) => {
    if (!resource.change?.actions) return false;
    return actions.some((action) => resource.change?.actions?.includes(action));
  });
}

/**
 * Get resources that will be created
 */
export function getCreatedResources(plan: Plan): ResourceChange[] {
  return getResourcesByAction(plan, "create");
}

/**
 * Get resources that will be updated
 */
export function getUpdatedResources(plan: Plan): ResourceChange[] {
  return getResourcesByAction(plan, "update");
}

/**
 * Get resources that will be deleted
 */
export function getDeletedResources(plan: Plan): ResourceChange[] {
  return getResourcesByAction(plan, "delete");
}

/**
 * Get resources that will be replaced (delete then create or create then delete)
 */
export function getReplacedResources(plan: Plan): ResourceChange[] {
  if (!plan.resource_changes) return [];

  return plan.resource_changes.filter((resource) => {
    if (!resource.change?.actions) return false;
    const actions = resource.change.actions;
    return actions.includes("delete") && actions.includes("create");
  });
}

/**
 * Get resources that have no changes (no-op)
 */
export function getUnchangedResources(plan: Plan): ResourceChange[] {
  return getResourcesByAction(plan, "no-op");
}

/**
 * Get resources by type
 */
export function getResourcesByType(plan: Plan, resourceType: string): ResourceChange[] {
  if (!plan.resource_changes) return [];

  return plan.resource_changes.filter((resource) => resource.type === resourceType);
}

/**
 * Get resources by provider
 */
export function getResourcesByProvider(plan: Plan, providerName: string): ResourceChange[] {
  if (!plan.resource_changes) return [];

  return plan.resource_changes.filter((resource) => resource.provider_name === providerName);
}

/**
 * Get resources by mode (managed or data)
 */
export function getResourcesByMode(plan: Plan, mode: "managed" | "data"): ResourceChange[] {
  if (!plan.resource_changes) return [];

  return plan.resource_changes.filter((resource) => resource.mode === mode);
}

/**
 * Get managed resources (excludes data sources)
 */
export function getManagedResources(plan: Plan): ResourceChange[] {
  return getResourcesByMode(plan, "managed");
}

/**
 * Get data source resources
 */
export function getDataSources(plan: Plan): ResourceChange[] {
  return getResourcesByMode(plan, "data");
}

/**
 * Get resources in a specific module
 */
export function getResourcesByModule(plan: Plan, moduleAddress?: string): ResourceChange[] {
  if (!plan.resource_changes) return [];

  // If moduleAddress is undefined or empty, get root module resources
  const targetModule = moduleAddress || "";

  return plan.resource_changes.filter((resource) => {
    const resourceModule = resource.module_address || "";
    return resourceModule === targetModule;
  });
}

/**
 * Get all output changes from the plan
 */
export function getOutputChanges(plan: Plan): Record<string, Change> {
  return plan.output_changes || {};
}

/**
 * Get outputs that will be created or updated
 */
export function getChangedOutputs(plan: Plan): Record<string, Change> {
  const outputs = getOutputChanges(plan);
  const changed: Record<string, Change> = {};

  for (const [name, change] of Object.entries(outputs)) {
    if (change.actions && !change.actions.includes("no-op")) {
      changed[name] = change;
    }
  }

  return changed;
}

/**
 * Get plan variables
 */
export function getPlanVariables(plan: Plan): Variables {
  return plan.variables || {};
}

/**
 * Get variable value by name
 */
export function getVariableValue(plan: Plan, variableName: string): any {
  const variables = getPlanVariables(plan);
  return variables[variableName]?.value;
}

/**
 * Get drifted resources (resources that changed outside of Terraform)
 */
export function getDriftedResources(plan: Plan): ResourceChange[] {
  return plan.resource_drift || [];
}

/**
 * Check if the plan has any changes
 */
export function planHasChanges(plan: Plan): boolean {
  const hasResourceChanges = !!(plan.resource_changes && plan.resource_changes.length > 0);
  const hasOutputChanges = !!(plan.output_changes && Object.keys(plan.output_changes).length > 0);

  return hasResourceChanges || hasOutputChanges;
}

/**
 * Check if the plan has any resource changes (excluding no-op)
 */
export function planHasResourceChanges(plan: Plan): boolean {
  if (!plan.resource_changes) return false;

  return plan.resource_changes.some((resource) => {
    const actions = resource.change?.actions || [];
    return !actions.includes("no-op") || actions.length === 0;
  });
}

/**
 * Get summary statistics about the plan
 */
export interface PlanSummary {
  total_resources: number;
  created: number;
  updated: number;
  deleted: number;
  replaced: number;
  unchanged: number;
  drifted: number;
  outputs_changed: number;
  variables_count: number;
  has_errors: boolean;
  success: boolean;
}

export function getPlanSummary(plan: Plan, success = true): PlanSummary {
  const created = getCreatedResources(plan);
  const updated = getUpdatedResources(plan);
  const deleted = getDeletedResources(plan);
  const replaced = getReplacedResources(plan);
  const unchanged = getUnchangedResources(plan);
  const drifted = getDriftedResources(plan);
  const changedOutputs = getChangedOutputs(plan);
  const variables = getPlanVariables(plan);

  return {
    total_resources: plan.resource_changes?.length || 0,
    created: created.length,
    updated: updated.length,
    deleted: deleted.length,
    replaced: replaced.length,
    unchanged: unchanged.length,
    drifted: drifted.length,
    outputs_changed: Object.keys(changedOutputs).length,
    variables_count: Object.keys(variables).length,
    has_errors: plan.errored || false,
    success,
  };
}

/**
 * Get all unique resource types in the plan
 */
export function getResourceTypes(plan: Plan): string[] {
  if (!plan.resource_changes) return [];

  const types = new Set<string>();
  for (const resource of plan.resource_changes) {
    if (resource.type) {
      types.add(resource.type);
    }
  }

  return Array.from(types).sort();
}

/**
 * Get all unique providers in the plan
 */
export function getProviders(plan: Plan): string[] {
  if (!plan.resource_changes) return [];

  const providers = new Set<string>();
  for (const resource of plan.resource_changes) {
    if (resource.provider_name) {
      providers.add(resource.provider_name);
    }
  }

  return Array.from(providers).sort();
}

/**
 * Get all module addresses referenced in the plan
 */
export function getModuleAddresses(plan: Plan): string[] {
  if (!plan.resource_changes) return [];

  const modules = new Set<string>();
  for (const resource of plan.resource_changes) {
    const moduleAddr = resource.module_address || "";
    modules.add(moduleAddr);
  }

  return Array.from(modules).sort();
}

/**
 * Find resources by name pattern (supports basic wildcard matching)
 */
export function findResourcesByName(plan: Plan, namePattern: string): ResourceChange[] {
  if (!plan.resource_changes) return [];

  // Convert simple wildcard pattern to regex
  const regex = new RegExp(namePattern.replace(/\*/g, ".*").replace(/\?/g, "."));

  return plan.resource_changes.filter((resource) => {
    if (!resource.name) return false;
    return regex.test(resource.name);
  });
}

/**
 * Find resources by address pattern
 */
export function findResourcesByAddress(plan: Plan, addressPattern: string): ResourceChange[] {
  if (!plan.resource_changes) return [];

  // Convert simple wildcard pattern to regex
  const regex = new RegExp(addressPattern.replace(/\*/g, ".*").replace(/\?/g, "."));

  return plan.resource_changes.filter((resource) => {
    if (!resource.address) return false;
    return regex.test(resource.address);
  });
}

/**
 * Get resources that are being imported
 */
export function getImportedResources(plan: Plan): ResourceChange[] {
  if (!plan.resource_changes) return [];

  return plan.resource_changes.filter((resource) => {
    return resource.change?.importing !== undefined;
  });
}

/**
 * Get resources that have generated config
 */
export function getResourcesWithGeneratedConfig(plan: Plan): ResourceChange[] {
  if (!plan.resource_changes) return [];

  return plan.resource_changes.filter((resource) => {
    return resource.change?.generated_config !== undefined;
  });
}

/**
 * Check if a resource has sensitive values
 */
export function resourceHasSensitiveValues(resource: ResourceChange): boolean {
  if (!resource.change) return false;

  const beforeSensitive = resource.change.before_sensitive;
  const afterSensitive = resource.change.after_sensitive;

  return (
    (beforeSensitive && Object.keys(beforeSensitive).length > 0) ||
    (afterSensitive && Object.keys(afterSensitive).length > 0)
  );
}

/**
 * Get all resources with sensitive values
 */
export function getResourcesWithSensitiveValues(plan: Plan): ResourceChange[] {
  if (!plan.resource_changes) return [];

  return plan.resource_changes.filter(resourceHasSensitiveValues);
}

/**
 * Get prior state root module resources
 */
export function getPriorStateResources(plan: Plan): Resource[] {
  if (!plan.prior_state?.root_module) return [];

  return getAllModuleResources(plan.prior_state.root_module);
}

/**
 * Recursively get all resources from a module and its children
 */
function getAllModuleResources(module: Module): Resource[] {
  const resources: Resource[] = [...(module.resources || [])];

  if (module.child_modules) {
    for (const childModule of module.child_modules) {
      resources.push(...getAllModuleResources(childModule));
    }
  }

  return resources;
}

/**
 * Get planned state root module resources
 */
export function getPlannedStateResources(plan: Plan): Resource[] {
  if (!plan.planned_values?.root_module) return [];

  return getAllModuleResources(plan.planned_values.root_module);
}

/**
 * Get prior state outputs
 */
export function getPriorStateOutputs(plan: Plan): Record<string, Output> {
  return plan.prior_state?.outputs || {};
}

/**
 * Find a resource in prior state by address
 */
export function findPriorStateResource(plan: Plan, address: string): Resource | undefined {
  const resources = getPriorStateResources(plan);
  return resources.find((r) => r.address === address);
}

/**
 * Compare a resource between prior and planned state
 */
export interface ResourceComparison {
  address: string;
  existsInPrior: boolean;
  existsInPlanned: boolean;
  priorResource?: Resource;
  plannedResource?: Resource;
  change?: ResourceChange;
}

export function compareResource(plan: Plan, address: string): ResourceComparison {
  const priorResource = findPriorStateResource(plan, address);
  const plannedResources = getPlannedStateResources(plan);
  const plannedResource = plannedResources.find((r) => r.address === address);
  const change = plan.resource_changes?.find((rc) => rc.address === address);

  return {
    address,
    existsInPrior: !!priorResource,
    existsInPlanned: !!plannedResource,
    priorResource,
    plannedResource,
    change,
  };
}

/**
 * Get resources that exist in prior state but not in planned state (being deleted)
 */
export function getResourcesBeingDeleted(plan: Plan): Resource[] {
  const priorResources = getPriorStateResources(plan);
  const plannedAddresses = new Set(getPlannedStateResources(plan).map((r) => r.address));
  
  return priorResources.filter((r) => !plannedAddresses.has(r.address));
}

/**
 * Get resources that exist in planned state but not in prior state (being created)
 */
export function getResourcesBeingCreated(plan: Plan): Resource[] {
  const plannedResources = getPlannedStateResources(plan);
  const priorAddresses = new Set(getPriorStateResources(plan).map((r) => r.address));
  
  return plannedResources.filter((r) => !priorAddresses.has(r.address));
}

/**
 * Check if plan has prior state
 */
export function hasPriorState(plan: Plan): boolean {
  return !!(plan.prior_state && 
    (plan.prior_state.root_module?.resources?.length || 
     Object.keys(plan.prior_state.outputs || {}).length));
}
