# Middleware Integration

## Summary

This RFC proposes adding middleware capabilities to OpenTofu that allow interception and modification of provider operations. Middleware would enable powerful features like cost tracking, approval gates, policy enforcement, and audit logging without requiring changes to individual providers.

_This work is based on the [original middleware RFC (#3016)](https://github.com/opentofu/opentofu/issues/3016). We recommend reading that RFC first for background context and motivation._

>[!NOTE]
> **Community feedback wanted!** Naming is hard, I'm not 100% certain about the name "Middleware", please help me name this by providing your feedback as a comment here in the pull request.

## Middleware as Provider Extension

Middleware is implemented as an additional capability that regular providers can optionally support alongside their resource management functionality.

This means the same provider process that manages your AWS resources can also provide middleware hooks for cost tracking, policy enforcement, or audit logging.

### Example: CompanyCo Internal Provider with Middleware

```hcl
terraform {
  required_providers {
    companyco = { 
      source = "companyco/internal-platform"
      version = "~> 2.1" 
    }
  }
}

# CompanyCo provider also serves middleware for governance
middleware "companyco" "cost_tracker" {
  budget_limit = 5000
  cost_center = "engineering"
  alert_channels = ["#platform-alerts"]
}

middleware "companyco" "security_policy" {
  require_team_ownership = true
  enforce_naming_convention = true
  required_environments = ["staging", "production"]
}

middleware "companyco" "change_approval" {
  production_requires_approval = true
  approvers = ["platform-team@companyco.com"]
  auto_approve_dev = true
}

provider "companyco" {
  api_endpoint = "https://platform.companyco.internal"
  
  # Use middleware from the same provider
  middleware = [
    provider.companyco.cost_tracker,
    provider.companyco.security_policy,
    provider.companyco.change_approval
  ]
}

# CompanyCo manages internal services and infrastructure
resource "companyco_application" "user_service" {
  name = "user-service"
  team = "platform"
  environment = "production"
  replicas = 3
}

resource "companyco_database" "user_db" {
  name = "user-service-db"
  application = companyco_application.user_service.name
  size = "medium"
}
```

### Technical Implementation

Middleware functionality extends the existing provider protocol with additional message types:
- **Middleware Hook Messages**: `["middleware_hook", {...}]` for processing middleware events
- **Metadata Response**: Return middleware metadata alongside standard responses
- **Hook Registration**: Providers declare which hooks they want to receive during initialization

The provider process handles both regular resource operations AND middleware hook processing, using the same MessagePack-RPC protocol defined in the [provider protocol document](./01-provider-protocol.md).

## What is Middleware?

Middleware in OpenTofu is the idea of reacting to events and extending the functionality of OpenTofu Core. This means reacting to resources being created, planning completing or apply failing for example, and then reacting to that with code written as part of a provider.

## Use Cases

### Cost Tracking and Budget Enforcement

Middleware could track the cost of infrastructure changes and enforce budget limits:

- Monitor estimated costs during planning each resource
  - Fail early if you know the plan is too expensive! No need to wait for the plan to complete.
- Prevent applies that exceed budget thresholds
- Generate cost reports and alerts after applying
- Track spending by team, project, or environment

### Policy Enforcement

Middleware could enforce organizational policies:

- Security policies (no public S3 buckets, encrypted storage required)
- Compliance requirements (tagging standards, naming conventions)
- Resource limits (maximum instance sizes, region restrictions)
- Integration with policy engines like OPA or Sentinel or jsPolicy.

### Audit and Compliance

Middleware could provide enhanced logging and audit trails:

- Detailed operation logs with metadata
- Change attribution and approval tracking
- Compliance reporting
- Integration with SIEM systems

### Integration with ITSM

Organizations could build custom middleware for specific needs:

- Store information about the generated resources into servicenow
- Trigger a backstage notification every time an apply fails
- Only allow applying if you have a change control ticket open in jira
  
## How Middleware Works

### Middleware Hook Points

Middleware operates at two distinct levels with different hook points:

#### Resource-Level Hooks

These hooks fire for each individual resource or data source:

- **`pre-plan`**: Before planning a specific resource
- **`post-plan`**: After planning a specific resource  
- **`pre-apply`**: Before applying changes to a specific resource
- **`post-apply`**: After applying changes to a specific resource
- **`pre-refresh`**: Before refreshing state for a specific resource
- **`post-refresh`**: After refreshing state for a specific resource

#### Operation-Level Hooks

These hooks fire for entire OpenTofu operations:

- **`init-stage-start`**: Before the init stage begins
- **`init-stage-complete`**: After the init stage completes successfully
- **`init-stage-fail`**: After the init stage fails
- **`plan-stage-start`**: Before the plan stage begins
- **`plan-stage-complete`**: After the plan stage completes successfully
- **`plan-stage-fail`**: After the plan stage fails
- **`apply-stage-start`**: Before the apply stage begins
- **`apply-stage-complete`**: After the apply stage completes successfully
- **`apply-stage-fail`**: After the apply stage fails

### Middleware Chain and Execution Order

Multiple middleware components can be chained together, executing in a defined order:

**Execution Order**:

1. **Global middleware** (from `terraform.middleware`) - runs for all operations
2. **Provider-specific middleware** (from `provider.middleware`) - runs only for that provider's resources

Middleware will be executed in the order in which it appears in the array of middleware. If the same middleware is passed both globally and per-provider it will be executed multiple times, each time overwriting the metadata of the last.

**Example Execution Flow**:
```hcl
terraform {
  middleware = [provider.cost.global_budget, provider.approval.gate]
}

provider "aws" {
  middleware = [provider.cost.aws_optimizer, provider.policy.aws_checker] 
}
```

For an AWS resource operation:
1. `provider.cost.global_budget` (global)
2. `provider.approval.gate` (global)  
3. `provider.cost.aws_optimizer` (AWS-specific)
4. `provider.policy.aws_checker` (AWS-specific)

Each middleware component has access to:
- The original operation data
- Metadata returned by previous middleware in the chain
- Current resource/operation context

### Middleware Metadata System

Middleware can attach metadata to resources that shall be persisted in both the plan and state files:

#### Metadata Structure
```json
{
  "__middleware_metadata__": {
    "cost_estimator": {
      "hourly": 0.102,
      "monthly": 73.58,
      "currency": "USD"
    },
    "approval_tracker": {
      "approved_by": "team-lead@company.com",
      "approval_timestamp": "2025-01-15T10:30:00Z"
    }
  }
}
```

#### Metadata Requirements
- Middleware returns an object that gets stored as `__middleware_metadata__.<MIDDLEWARENAME>`
- Metadata is read-only and cannot modify resource configurations
- Metadata persists across plan/apply cycles
- Metadata is accessible to subsequent middleware in the chain
- Metadata should be stored in both the plan file and the state file so external tooling can run against it

### Middleware Response Format

Middleware responds with the following data:

```json
{
  "status": "success",  // "success", "fail", or "warning"
  "message": "Cost estimate: $73.58/month",
  "metadata": {
    "estimated_cost": {
      "hourly": 0.102,
      "monthly": 73.58,
      "currency": "USD"
    }
  }
}
```

**Response Options**:
- **`success`**: Allow operation to continue
- **`fail`**: Block operation and return error
- **`warning`**: Allow operation but display warning

### Error Handling and Failures

When middleware returns a "fail" status:
- The operation is immediately halted
- The error message is displayed to the user
- No subsequent middleware in the chain executes
- The resource operation is not performed

### Configuration Model

Middleware is provided by providers and configured using a provider-based model similar to regular providers.

#### Configuration Syntax

**Middleware Declaration**: `middleware "providername" "middlewarename" { }`
- `providername` must be declared in `required_providers`
- `middlewarename` is a local name for this middleware instance
- Configuration block contains middleware-specific settings

**Global Middleware**: `terraform { middleware = [...] }`
- Runs for all providers and resources
- Configured in the `terraform` block alongside `required_providers`

**Provider-Specific Middleware**: `provider "name" { middleware = [...] }`
- Runs only for resources from that specific provider
- Configured in the provider block

## Implementation Considerations

### Sensitive Values and Security

#### Sensitive Data Handling

Middleware interaction with sensitive values could be controlled through configuration:

```hcl
middleware "audit" "logger" {
  log_destination = "splunk"
  
  # Sensitive value handling
  send_sensitive = false    # Don't send sensitive values to middleware
  sanitize_logs = true     # Remove sensitive data from logs
}
```

**Sensitive Value Modes**:
- **`send_sensitive = true`**: Middleware receives all values including sensitive ones
- **`send_sensitive = false`**: Sensitive values are redacted before sending to middleware
- **Default**: `false` for security by default

#### Security Considerations

**Read-Only Architecture**:
- Middleware cannot directly modify resource configurations
- Middleware cannot directly manipulate state files, only attach metadata
- All changes go through OpenTofu's standard validation and storage

### Performance Impact

Middleware adds overhead to operations:

- Each hook adds latency
- Complex middleware could slow operations significantly  
- Need mechanisms to bypass middleware for emergency situations
- Consider async middleware for non-blocking operations

## Future Possibilities

### Advanced Features

- **Conditional Middleware**: Apply middleware based on environment, user, or change type
- **Middleware Dependencies**: Middleware that depends on other middleware
- **Dynamic Configuration**: Middleware configuration that changes based on context
- **Remote Middleware**: Middleware that runs as external services

### Ecosystem Development

- **Middleware Marketplace**: Registry of available middleware components
- **Standard Library**: Common middleware patterns and implementations
- **Integration Frameworks**: Easy integration with external systems
- **Testing Tools**: Tools for testing and debugging middleware

## Conclusion

Middleware integration would provide OpenTofu with powerful governance, compliance, and workflow capabilities while maintaining the simplicity and flexibility that makes OpenTofu valuable. By allowing interception and modification of provider operations, middleware enables organizations to implement custom business logic and controls without requiring changes to core OpenTofu or individual providers.
