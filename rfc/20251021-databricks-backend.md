# Databricks Backend with Locking for OpenTofu

This RFC proposes adding Databricks as a new state backend for OpenTofu, enabling teams using Databricks with limited access to Cloud Service Providers to manage their infrastructure state. The backend utilizes Unity Catalog volumes for state storage, offering flexible locking options: [Databricks Lakebase](https://docs.databricks.com/aws/en/oltp/instances/about) (a managed PostgreSQL instance) or [Delta Lake](https://docs.delta.io/) tables as a fallback.

### Motivation

- **Dissuasion to implement IaC** due to a lack of cloud storage backends  
- **Duplicate access control systems** (cloud IAM vs. Unity Catalog RBAC/ABAC)  
- **Separate audit trails** requiring manual correlation  
- **Increased operational complexity** for teams managing both infrastructure and data

Unity Catalog provides a unified governance layer for data assets with sophisticated access controls, comprehensive audit logging, and tight integration with Databricks workspaces. By extending this governance model to infrastructure state management, organizations can achieve a single source of truth for both data and infrastructure operations.

### Use Cases

1. **Unified Governance**: Organizations requiring a single audit trail for compliance  
2. **Databricks-Native Infrastructure**: Teams managing primarily Databricks resources  
3. **Multi-Workspace Management**: Consistent state management across Databricks workspaces  
4. **Enterprise Compliance**: Regulated industries needing comprehensive governance

## Proposed Solution

The backend utilizes a hybrid storage model, with state files stored in Unity Catalog volumes and locking implemented via either Lakebase or Delta Lake tables. Authentication supports Databricks [Unified Auth](https://docs.databricks.com/aws/en/dev-tools/auth/unified-auth).

### User Documentation

#### Backend Configuration

```hcl
terraform {
  backend "databricks" {
    # Required: Databricks connection
    host = "https://workspace.cloud.databricks.com"

    # Required: Unity Catalog location
    catalog = "terraform_state"
    schema  = "production"
    volume  = "infrastructure"

    # Required: State file path
    key = "terraform.tfstate"

    # Optional: Locking configuration (defaults to Delta)
    lock_backend = "lakebase"  # or "delta"
    lock_timeout = "10m"

    # Optional: Authentication (uses SDK auth chain if omitted)
    token = "${env:DATABRICKS_TOKEN}"
  }
}
```

#### State Storage

State files are stored as JSON blobs in Unity Catalog volumes:

- **Path format**: `/Volumes/{catalog}/{schema}/{volume}/{key}`  
- **Access**: Databricks Files API/SDK  
- **Versioning**: Leverages underlying cloud storage  
- **Encryption**: Cloud provider encryption \+ optional client-side encryption

#### State Locking

**Option 1: Lakebase Locking (Recommended)**

[Databricks Lakebase](https://docs.databricks.com/en/oltp/) provides PostgreSQL-compatible locking with:

- ACID transaction guarantees  
- Sub-50-ms lock acquisition performance  
- Higher cost due to compute requirements

**Option 2: Delta Lake Locking (Fallback)**

For environments without Lakebase, Delta Lake tables provide locking via primary key constraints:

- ACID transaction guarantees  
- Higher latency (\~1 second)  
- May require a SQL Warehouse to execute statements (higher cost)  
- May be able to use [delta-kernel-rs](https://github.com/delta-io/delta-kernel-rs) to access (very cheap)

#### Authentication

Supports Databricks SDK [authentication chain](https://docs.databricks.com/en/dev-tools/auth/unified-auth):

1. Explicit configuration (token, service principal)  
2. Environment variables  
3. Configuration file (`~/.databrickscfg`)

#### Migration from Existing Backends

```shell
# 1. Create Unity Catalog resources
databricks catalogs create terraform_state
databricks schemas create production terraform_state
databricks volumes create terraform_state production \
    infrastructure MANAGED

# 2. Update backend configuration
# 3. Migrate state
tofu init -migrate-state
```

### Technical Approach

The backend implements the standard OpenTofu backend interface in `internal/backend/remote-state/databricks`:

```go
package databricks

type Backend struct {
    client      *databricks.WorkspaceClient
    catalog     string
    schema      string
    volume      string
    lockBackend string  // "lakebase" or "delta"
}

// Standard backend methods
func (b *Backend) StateMgr(workspace string) (statemgr.Full, error)
func (b *Backend) Workspaces() ([]string, error)
func (b *Backend) DeleteWorkspace(name string, force bool) error
```

#### Development Phases

1. **Core Implementation**: Backend interface, basic state operations  
2. **Locking Implementation**: Lakebase and Delta Lake locking  
3. **Production Hardening**: Performance optimization, comprehensive error handling  
4. **Release Preparation**: Documentation, migration tools, benchmarks

#### Testing Strategy

- **Unit Tests**: Backend interface methods (>80% coverage)  
- **Integration Tests**: Real Unity Catalog instance  
- **Stress Tests**: Concurrent operations, lock contention  
- **Performance Tests**: Meet latency targets

#### Performance Targets

| Operation | Target (p95) |
| :---- | :---- |
| State GET | < 3 seconds |
| State PUT | < 5 seconds |
| Lock acquisition (Lakebase) | < 50ms |
| Lock acquisition (Delta) | < 2 seconds |

#### Error Handling

Implements retry logic with exponential backoff for transient failures:

- Network errors: 3 retries  
- Rate limiting (429): Automatic backoff  
- Authentication failures: No retry

### Open Questions

#### Technical

1. Should Postgres via Lakebase be required or optional with Delta fallback?  
2. Should the backend auto-create lock tables?

#### Product

1. Should this be a plugin or a core backend?

### Future Considerations

- State caching to reduce API calls  
- Native Databricks monitoring integration  
- Support for using Iceberg as the lock table format

## Potential Alternatives

### 1. Lakebase / Lakehouse Federation Backend

Use Lakebase for both state and locks without Unity Catalog volumes.

**Pros**: Simpler, proven PostgreSQL patterns  
**Cons**: No Unity Catalog governance integration, and expensive to continuously run compute

### 2. Delta Lake Tables for State

Store state directly in Delta tables instead of volumes.

**Pros**: ACID guarantees, time travel  
**Cons**: Higher latency. Changes to state serialization (e.g., JSON written to VARIANT column)

### 3. Traditional Backend with UC Metadata

Continue using S3/Azure/GCS with Unity Catalog metadata tracking.

**Pros**: Proven reliability, lower cost  
**Cons**: Doesn't unify governance

## Locking Comparison

| Aspect | Lakebase | Delta Lake |
| :---- | :---- | :---- |
| **Atomic Operations** | Native PostgreSQL | PK constraint |
| **Performance** | \~50ms | \~2 seconds |
| **Cost** | Instance pricing | SQL warehouse compute (unless delta kernel is feasible) |
| **Setup Complexity** | Create a Managed instance | Use existing Delta |

## Prior Art

- [**S3 Backend**](https://github.com/opentofu/opentofu/tree/main/internal/backend/remote-state/s3): DynamoDB or native S3 locking  
- [**Azure Backend**](https://github.com/opentofu/opentofu/tree/main/internal/backend/remote-state/azure): Blob leases for locking  
- [**GCS Backend**](https://github.com/opentofu/opentofu/tree/main/internal/backend/remote-state/gcs): Preconditions for consistency  
- [**PostgreSQL Backend**](https://github.com/opentofu/opentofu/tree/main/internal/backend/remote-state/pg): Database-based state and locking

No existing Databricks backend implementations were found in Terraform/OpenTofu ecosystems.  