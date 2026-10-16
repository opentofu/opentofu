# Registry Integration

## Summary

This document defines a simplified registry integration model for the new proposed OpenTofu providers focused on **discovery rather than distribution**. The registry serves as a lightweight catalog that helps users find providers and learn how to use them, while leaving versioning, installation, and distribution to external systems (package managers, version control, etc.).

## Design Philosophy: Discovery Over Distribution

### Core Principle

The registry's primary purpose for these new providers is **provider discovery and documentation**, not complex version management or binary distribution. Users discover providers through the registry, then follow installation instructions from the provider's documentation.

### Why This Approach

1. **Simplicity**: Avoids complex versioning, security, and distribution infrastructure
2. **Flexibility**: Providers can use any distribution method (npm, PyPI, Docker, Git, etc.)
3. **Security**: Eliminates registry as a potential attack vector for malicious code execution
4. **Maintenance**: Minimal registry infrastructure and maintenance burden
5. **Innovation**: Allows experimentation with different distribution approaches

### Relationship to Existing Registry

This proposal coexists with the existing OpenTofu registry:
- **Existing registry**: Continues to serve traditional binary providers with full version management
- **New registry section**: Adds a lightweight discovery-only section for local execution providers
- **User choice**: Users can choose between traditional providers (complex, secure) and local providers (simple, flexible)

## Registry Metadata Format

### Minimal Provider Entry

```json
{
  "name": "myapp",
  "namespace": "myorg", 
  "description": "Internal provider for pmyapp",
  "repository": {
    "type": "github",
    "url": "https://github.com/myorg/myapp-provider"
  },
  "documentation": {
    "readme": "https://github.com/myorg/myapp-provider/blob/main/README.md"
  },
  "tags": ["governance", "policy", "internal"],
  "created_at": "2025-01-15T10:30:00Z",
  "updated_at": "2025-01-20T14:22:00Z"
}
```

### Required Fields

- **`name`**: Provider name (used in `required_providers`)
- **`namespace`**: Organization or user namespace  
- **`description`**: Brief description of provider functionality
- **`repository.url`**: Link to source repository
- **`documentation.readme`**: Link to installation and usage documentation

### Optional Fields

- **`repository.type`**: Repository type (`github`, `gitlab`, `bitbucket`, `git`)
- **`tags`**: Searchable keywords
- **`created_at`** / **`updated_at`**: Timestamps for registry management

### What's NOT Included

- **No version information**: Registry is versionless
- **No download URLs**: Users get software from repository or package managers
- **No checksums**: Integrity handled by external systems
- **No dependency information**: Dependencies managed by package managers
- **No execution metadata**: cmd+args specified by users locally

## Versionless Provider Model

### How It Works

```hcl
terraform {
  required_providers {
    # User discovers provider in registry
    governance = {
      # Registry provides repository URL, user follows README for installation
      cmd  = "python3"  
      args = ["./governance-provider.py", "--stdio"]
    }
  }
}
```

### Benefits of Versionless Registry

1. **Eliminates Version Conflicts**: No complex dependency resolution in registry
2. **Flexibility**: Providers can use any versioning scheme they choose
3. **Immediate Updates**: Provider updates don't require registry submissions
4. **Reduced Complexity**: Registry doesn't need to understand different versioning systems
5. **User Control**: Users explicitly choose which version/installation method to use

## Discovery Workflow

### Registry API
< REDO ALL OF THIS, PROPOSE A new v2 api maybe? say that it's unclear right now how or if we should communicate back to the opentofu binary>

## Provider Submission Process

The review process should be very similar to what we have today however require more fields from the form. Due to the nature of the new providers, we cannot infer as much information as we currently do. The flexibility given to the provider developers does negatively impact the registry slightly but this just requires a tiny amount more of human steps. 

Note: There are existing tools in the MCP marketplace ecosystem that will read the README.md using an LLM and use that to infer specifics, this is an approach we could adopt in the long term.

### Namespace Management

By allowing provider authors to submit new providers from non-github sources, we are breaking the concept of a namespace being a 1 to 1 mapping to a github organization. We should discuss a way to allow for us to decouple these and allow people to define namespaces in a first-come-first-serve basis. Similar to other projects (dockerhub, npm, etc).

## Future Enhancements

### Potential Future Additions

**Enhanced Metadata** (if valuable):
- Provider category/type classification
- Minimum OpenTofu version requirements
- Provider protocol version
- Example configuration snippets

**Community Features** (if needed):
- Provider ratings/reviews
- Download/usage statistics
- Provider health monitoring
- Community discussion integration

**Registry Federation** (if ecosystem grows):
- Support for multiple registry sources
- Private/internal registry mirrors
- Registry synchronization protocols

### What We Shouldn't Add

**Complex Features to Avoid**:
- Version dependency resolution
- Binary/package distribution
- Security scanning or verification
- Complex approval workflows
- Usage analytics or tracking


## Versioning approaches

I propose that to get started we should start versionless and discuss with the community through RFC processes about how we can intoroduce versioning to this process.

## Conclusion

This registry integration model prioritizes simplicity and discovery over complex distribution mechanics. By focusing on helping users find and learn about providers rather than managing their installation and versioning, we create a lightweight system that supports the diverse needs of the OpenTofu provider ecosystem while maintaining security and flexibility.

The versionless approach eliminates many common registry problems (dependency hell, version conflicts, complex resolution) while empowering providers to use whatever distribution and versioning approach works best for their use case.
