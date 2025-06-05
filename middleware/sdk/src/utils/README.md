# Plan Utilities

The `plan.ts` utilities provide helper functions for analyzing OpenTofu plan files in middleware. These functions make it easy to extract and filter resources, outputs, and other plan data.

## Basic Usage

```typescript
import { getPlanSummary, getCreatedResources, getUpdatedResources } from "@opentofu/middleware";

// In your onPlanCompleted handler
.onPlanCompleted((params) => {
  const plan = params.plan_json;
  
  // Get a summary of all changes
  const summary = getPlanSummary(plan, params.success);
  console.log(`${summary.created} resources will be created`);
  
  // Get specific resource changes
  const created = getCreatedResources(plan);
  const updated = getUpdatedResources(plan);
  
  return { status: "pass" };
})
```

## Resource Filtering

### By Action Type
- `getCreatedResources(plan)` - Resources to be created
- `getUpdatedResources(plan)` - Resources to be updated  
- `getDeletedResources(plan)` - Resources to be deleted
- `getReplacedResources(plan)` - Resources to be replaced
- `getUnchangedResources(plan)` - Resources with no changes

### By Resource Properties
- `getResourcesByType(plan, "aws_instance")` - Filter by resource type
- `getResourcesByProvider(plan, "registry.opentofu.org/hashicorp/aws")` - Filter by provider
- `getManagedResources(plan)` - Get only managed resources (exclude data sources)
- `getDataSources(plan)` - Get only data source resources
- `getResourcesByModule(plan, "module.web")` - Filter by module

### Pattern Matching
- `findResourcesByName(plan, "web-*")` - Find resources by name pattern (supports wildcards)
- `findResourcesByAddress(plan, "*.aws_instance.*")` - Find by address pattern

## Special Resource Types

- `getImportedResources(plan)` - Resources being imported
- `getResourcesWithGeneratedConfig(plan)` - Resources with generated configuration
- `getResourcesWithSensitiveValues(plan)` - Resources containing sensitive data
- `getDriftedResources(plan)` - Resources that drifted outside of Terraform

## Outputs and Variables

- `getOutputChanges(plan)` - All output changes
- `getChangedOutputs(plan)` - Only outputs that are changing (not no-op)
- `getPlanVariables(plan)` - All plan variables
- `getVariableValue(plan, "instance_type")` - Get specific variable value

## Plan Analysis

- `planHasChanges(plan)` - Check if plan has any changes
- `planHasResourceChanges(plan)` - Check if plan has resource changes (excluding no-op)
- `getPlanSummary(plan, success)` - Get comprehensive plan statistics

### Plan Summary

The `getPlanSummary()` function returns a `PlanSummary` object with:

```typescript
interface PlanSummary {
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
```

## Discovery Functions

- `getResourceTypes(plan)` - Get all unique resource types in the plan
- `getProviders(plan)` - Get all unique providers in the plan  
- `getModuleAddresses(plan)` - Get all module addresses referenced

## State Analysis

- `hasPriorState(plan)` - Check if plan has prior state data
- `getPriorStateResources(plan)` - Resources from prior state
- `getPlannedStateResources(plan)` - Resources in planned state
- `getPriorStateOutputs(plan)` - Outputs from prior state
- `findPriorStateResource(plan, address)` - Find specific resource in prior state
- `compareResource(plan, address)` - Compare resource between prior and planned state
- `getResourcesBeingDeleted(plan)` - Resources in prior but not planned state
- `getResourcesBeingCreated(plan)` - Resources in planned but not prior state

## Examples

### Cost Analysis

```typescript
.onPlanCompleted((params) => {
  const plan = params.plan_json;
  
  // Find resources that will incur costs
  const costIncurringResources = [
    ...getCreatedResources(plan),
    ...getUpdatedResources(plan)
  ];
  
  // Analyze by cloud provider
  const awsResources = getResourcesByProvider(plan, "registry.opentofu.org/hashicorp/aws");
  const gcpResources = getResourcesByProvider(plan, "registry.opentofu.org/hashicorp/google");
  
  console.log(`AWS: ${awsResources.length}, GCP: ${gcpResources.length}`);
  
  return { status: "pass" };
})
```

### Security Analysis

```typescript
.onPlanCompleted((params) => {
  const plan = params.plan_json;
  
  // Find resources with sensitive data
  const sensitiveResources = getResourcesWithSensitiveValues(plan);
  
  // Check for public resources
  const publicSubnets = findResourcesByName(plan, "*public*");
  
  if (sensitiveResources.length > 0) {
    console.warn(`Found ${sensitiveResources.length} resources with sensitive values`);
  }
  
  return { status: "pass" };
})
```

### Compliance Checking

```typescript
.onPlanCompleted((params) => {
  const plan = params.plan_json;
  
  // Get all S3 buckets
  const s3Buckets = getResourcesByType(plan, "aws_s3_bucket");
  
  // Check for untagged resources
  const created = getCreatedResources(plan);
  const untaggedResources = created.filter(resource => {
    // Check if resource has required tags
    const config = resource.change?.after;
    return !config?.tags || !config.tags.Environment;
  });
  
  if (untaggedResources.length > 0) {
    return {
      status: "fail",
      message: `${untaggedResources.length} resources missing required tags`
    };
  }
  
  return { status: "pass" };
})
```

### Prior State Analysis

```typescript
.onPlanCompleted((params) => {
  const plan = params.plan_json;
  
  if (!hasPriorState(plan)) {
    console.log("This is the first run - no prior state exists");
    return { status: "pass" };
  }
  
  // Compare resource changes
  const resources = plan.resource_changes || [];
  
  for (const resourceChange of resources) {
    const comparison = compareResource(plan, resourceChange.address);
    
    if (comparison.existsInPrior && comparison.existsInPlanned) {
      // Resource is being modified
      console.log(`Resource ${resourceChange.address} is being modified`);
      console.log(`Prior values:`, comparison.priorResource?.values);
      console.log(`Planned values:`, comparison.plannedResource?.values);
    } else if (comparison.existsInPrior && !comparison.existsInPlanned) {
      // Resource is being deleted
      console.log(`Resource ${resourceChange.address} is being deleted`);
    } else if (!comparison.existsInPrior && comparison.existsInPlanned) {
      // Resource is being created
      console.log(`Resource ${resourceChange.address} is new`);
    }
  }
  
  // Check for resources being replaced
  const replacedResources = getReplacedResources(plan);
  for (const resource of replacedResources) {
    const priorResource = findPriorStateResource(plan, resource.address);
    if (priorResource) {
      console.log(`Resource ${resource.address} is being replaced`);
      console.log(`Old ID: ${priorResource.values?.id}`);
    }
  }
  
  return { status: "pass" };
})
```