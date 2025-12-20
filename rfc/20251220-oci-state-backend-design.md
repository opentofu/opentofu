# OCI State Backend: Design and Implementation

**Date:** December 20, 2025  
**Status:** Implemented  
**Author:** Victor  

---

## Abstract

This document specifies the design and implementation of the OCI registry backend for OpenTofu state storage. The implementation enables users to store their infrastructure state in any OCI-compliant registry, leveraging existing organizational infrastructure rather than requiring dedicated storage systems.

---

## 1. Motivation

### 1.1 Why State Storage First?

OpenTofu users need to store sensitive infrastructure state data. This state contains:
- Resource identifiers and ARNs
- Network topology information
- Sensitive outputs and credentials references
- Infrastructure dependency graphs

**This is user data, not software.** It must remain private and under user control.

### 1.2 The User Sovereignty Principle

Many organizations already operate OCI-compliant registries:
- Harbor for internal container images
- GitHub Container Registry (GHCR) for CI/CD artifacts
- AWS ECR, Azure ACR, Google GCR for cloud workloads
- Self-hosted registries for air-gapped environments

**Our philosophy:** If users have storage infrastructure, let them use it. Don't force adoption of additional systems just to run OpenTofu.

### 1.3 Why OCI Registries?

| Benefit | Description |
|---------|-------------|
| **Ubiquity** | OCI registries exist in virtually every organization |
| **Security** | Built-in access control, audit logs, and encryption at rest |
| **Tooling** | Existing backup, replication, and disaster recovery |
| **No vendor lock-in** | Any OCI-compliant registry works |
| **Zero additional cost** | Leverages existing infrastructure investment |

### 1.4 Comparison with Existing Backends

| Backend | Requires | User Already Has |
|---------|----------|------------------|
| S3 | AWS account + bucket | Maybe |
| Azure Blob | Azure account + storage | Maybe |
| GCS | GCP account + bucket | Maybe |
| Consul | Consul cluster | Rarely |
| PostgreSQL | Database server | Sometimes |
| **OCI** | Any container registry | **Almost certainly** |

The OCI backend meets users where they are.

---

## 2. Design Overview

### 2.1 Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    OpenTofu CLI                         │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│              OCI State Backend                          │
│  ┌─────────────────┐  ┌─────────────────┐               │
│  │   backend.go    │  │    client.go    │               │
│  │  Configuration  │  │  State & Lock   │               │
│  │  Credentials    │  │   Operations    │               │
│  └─────────────────┘  └─────────────────┘               │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│              ORAS Client Library                        │
│         (OCI Registry As Storage)                       │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│              OCI Registry                               │
│   Harbor │ GHCR │ ECR │ ACR │ GCR │ Docker Hub │ ...    │
└─────────────────────────────────────────────────────────┘
```

### 2.2 Core Components

| Component | File | Responsibility |
|-----------|------|----------------|
| Backend | `backend.go` | Configuration, initialization, credential management |
| Client | `client.go` | State operations, locking, manifest handling |
| State Manager | `backend_state.go` | Interface implementation for OpenTofu |

### 2.3 Configuration

```hcl
terraform {
  backend "oci" {
    # Required: OCI repository path (without tag or digest)
    # Can also be set via TF_BACKEND_OCI_REPOSITORY environment variable
    repository = "registry.example.com/infrastructure/tofu-state"
    
    # Optional: for self-signed certificates
    ca_file = "/path/to/ca-bundle.crt"
    
    # Optional: for development only (not recommended)
    insecure = false
  }
}
```

**Environment Variables:**

| Variable | Description |
|----------|-------------|
| `TF_BACKEND_OCI_REPOSITORY` | Alternative to `repository` attribute. Useful for CI/CD pipelines. |

---

## 3. Artifact Layout

### 3.1 OCI Manifest Structure

State is stored as ORAS-compatible single-image manifests:

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "artifactType": "application/vnd.opentofu.state.v1",
  "config": {
    "mediaType": "application/vnd.oci.empty.v1+json",
    "digest": "sha256:44136fa...",
    "size": 2
  },
  "layers": [
    {
      "mediaType": "application/vnd.opentofu.statefile.v1",
      "digest": "sha256:abc123...",
      "size": 12345
    }
  ],
  "annotations": {
    "org.opentofu.workspace": "production"
  }
}
```

### 3.2 Tag Naming Convention

```
Repository: registry.example.com/infrastructure/tofu-state
│
├── state-default          # State for "default" workspace
├── state-production       # State for "production" workspace  
├── state-staging          # State for "staging" workspace
│
├── locked-default         # Lock artifact for "default" workspace
├── locked-production      # Lock artifact for "production" workspace
│
└── unlocked-default       # Fallback for registries without DELETE support
```

### 3.3 Workspace Name Handling

Valid OCI tags have restrictions. For workspace names that aren't valid tags:

```go
func workspaceTagFor(workspace string) string {
    // If workspace name is a valid OCI tag, use it directly
    ref := orasRegistry.Reference{Reference: workspace}
    if err := ref.ValidateReferenceAsTag(); err == nil {
        return workspace
    }
    
    // Otherwise, hash it to create a valid tag
    h := sha256.Sum256([]byte(workspace))
    return "ws-" + hex.EncodeToString(h[:8])
}
```

### 3.4 Artifact Types

| Artifact Type | Media Type | Purpose |
|---------------|------------|---------|
| State | `application/vnd.opentofu.state.v1` | Infrastructure state data |
| Lock | `application/vnd.opentofu.lock.v1` | Workspace lock information |
| State File | `application/vnd.opentofu.statefile.v1` | Actual state content (layer) |
| Lock Info | `application/vnd.opentofu.lockinfo.v1` | Lock metadata (layer) |

---

## 4. State Operations

### 4.1 Get State

```go
func (c *RemoteClient) Get(ctx context.Context) (*remote.Payload, error) {
    // 1. Resolve state tag to manifest descriptor
    // 2. Fetch manifest
    // 3. Extract state blob from layers
    // 4. Return payload with MD5 checksum
}
```

**Flow:**
1. Resolve `state-<workspace>` tag → manifest digest
2. Fetch manifest JSON
3. Extract layer[0] digest (state content)
4. Fetch blob by digest
5. Calculate MD5 for lineage tracking
6. Return state bytes

### 4.2 Put State

```go
func (c *RemoteClient) Put(ctx context.Context, data []byte) error {
    // 1. Push state blob to registry
    // 2. Create manifest referencing blob
    // 3. Tag manifest with workspace name
}
```

**Flow:**
1. Push state bytes as blob → get digest
2. Construct manifest with artifact type and annotations
3. Push manifest → get manifest digest
4. Tag manifest digest as `state-<workspace>`

### 4.3 Delete State

```go
func (c *RemoteClient) Delete(ctx context.Context) error {
    // 1. Resolve state tag
    // 2. Delete manifest (removes tag reference)
}
```

---

## 5. Locking Mechanism

### 5.1 Lock Acquisition

```go
func (c *RemoteClient) Lock(ctx context.Context, info *statemgr.LockInfo) (string, error) {
    // 1. Check if lock exists
    // 2. If not, create lock artifact
    // 3. Tag it as locked-<workspace>
    // 4. Return lock ID
}
```

**Lock Info Structure:**
```json
{
  "ID": "unique-lock-id",
  "Operation": "OperationTypeApply",
  "Info": "user@hostname",
  "Who": "user@hostname",
  "Version": "1.0.0",
  "Created": "2025-12-20T10:00:00Z",
  "Path": "registry.example.com/infrastructure/tofu-state"
}
```

### 5.2 Lock Release

```go
func (c *RemoteClient) Unlock(ctx context.Context, id string) error {
    // 1. Verify lock ID matches
    // 2. Delete lock artifact
    // 3. Handle registries that don't support DELETE (GHCR)
}
```

**GHCR Compatibility:** GitHub Container Registry returns `405 Method Not Allowed` for DELETE operations. The implementation handles this by pushing an "unlocked" marker instead:

```go
if isMethodNotAllowed(err) {
    // Push empty "unlocked" manifest as alternative
    return c.pushUnlockedMarker(ctx)
}
```

### 5.3 Lock Contention

When a lock already exists:
```
Error: Error acquiring the state lock

Error message: state is locked by another process
Lock Info:
  ID:        abc123
  Path:      registry.example.com/infrastructure/tofu-state
  Operation: OperationTypeApply
  Who:       alice@workstation
  Created:   2025-12-20 10:00:00 +0000 UTC

OpenTofu acquires a state lock to protect the state from being written
by multiple users at the same time. Please resolve the issue above and try
again. If the lock is stale, you can force-unlock using:
  tofu force-unlock abc123
```

---

## 6. Authentication

### 6.1 Credential Sources

The backend supports multiple credential sources in order of specificity:

1. **OpenTofu CLI configuration** (`~/.tofurc` or `terraform.rc`)
   ```hcl
   credentials "registry.example.com" {
     token = "..."
   }
   ```

2. **Docker credential helpers** (`~/.docker/config.json`)
   ```json
   {
     "credHelpers": {
       "registry.example.com": "ecr-login"
     }
   }
   ```

3. **Docker credentials file**
   ```json
   {
     "auths": {
       "registry.example.com": {
         "auth": "base64(username:password)"
       }
     }
   }
   ```

4. **Environment variables** (registry-specific)
   - `AWS_*` for ECR
   - `GOOGLE_APPLICATION_CREDENTIALS` for GCR
   - etc.

### 6.2 Credential Selection

```go
func (p *credentialsPolicy) CredentialFunc(ctx context.Context, repository string) (credentialFunc, error) {
    // 1. Parse repository to extract registry domain
    // 2. Find most specific credential source
    // 3. Return credential function for ORAS client
}
```

**Specificity Rules:**
- Exact repository match > registry match > default
- OpenTofu config > Docker helpers > Docker auth file

### 6.3 Supported Registries

| Registry | Authentication Method |
|----------|----------------------|
| Harbor | Username/password, OIDC |
| GHCR | Personal access token |
| AWS ECR | IAM credentials via helper |
| Azure ACR | Service principal, managed identity |
| Google GCR | Service account JSON |
| Docker Hub | Username/password |
| Self-hosted | Configurable |

---

## 7. HTTP Client Configuration

### 7.1 TLS Configuration

```go
func newOCIHTTPClient(ctx context.Context, insecure bool, caFile string) (*http.Client, error) {
    client := cleanhttp.DefaultPooledClient()
    transport := client.Transport.(*http.Transport)
    
    // Custom CA bundle for self-signed certificates
    if caFile != "" {
        pool := x509.NewCertPool()
        pem, _ := os.ReadFile(caFile)
        pool.AppendCertsFromPEM(pem)
        transport.TLSClientConfig.RootCAs = pool
    }
    
    // Development mode (not recommended for production)
    if insecure {
        transport.TLSClientConfig.InsecureSkipVerify = true
    }
    
    return client, nil
}
```

### 7.2 Observability

The HTTP client is instrumented with OpenTelemetry:

```go
if span := tracing.SpanFromContext(ctx); span != nil && span.IsRecording() {
    transport = otelhttp.NewTransport(transport)
}
```

This enables:
- Distributed tracing across registry calls
- Latency metrics for registry operations
- Error tracking and debugging

### 7.3 User-Agent

All requests include a proper User-Agent header:
```
User-Agent: OpenTofu/1.x.x (+https://opentofu.org)
```

---

## 8. Error Handling

### 8.1 Error Categories

| Category | Examples | Handling |
|----------|----------|----------|
| Not Found | State doesn't exist yet | Return nil (valid case) |
| Unauthorized | Invalid credentials | Clear error message |
| Forbidden | Insufficient permissions | Clear error message |
| Network | Connection timeout | Propagate with context |
| Registry | 5xx errors | Propagate with context |

### 8.2 Error Detection

```go
func isNotFound(err error) bool {
    var errResp *errcode.ErrorResponse
    if errors.As(err, &errResp) {
        for _, e := range errResp.Errors {
            if e.Code == errcode.ErrorCodeManifestUnknown ||
               e.Code == errcode.ErrorCodeNameUnknown {
                return true
            }
        }
    }
    return false
}
```

---

## 9. Testing

### 9.1 Unit Tests

The implementation includes unit tests with a mock OCI repository:

```go
type fakeOCIRepo struct {
    manifests map[string][]byte
    blobs     map[string][]byte
    tags      map[string]string
}

func (f *fakeOCIRepo) Push(ctx context.Context, desc ocispec.Descriptor, content io.Reader) error {
    // Store blob in memory
}

func (f *fakeOCIRepo) Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error) {
    // Resolve tag to descriptor
}
```

### 9.2 Test Coverage

| Scenario | Test |
|----------|------|
| State read/write | `TestRemoteClient_GetPut` |
| Lock acquisition | `TestRemoteClient_Lock` |
| Lock contention | `TestRemoteClient_LockContentionAndUnlockMismatch` |
| Workspace naming | `TestWorkspaceTagFor` |
| GHCR compatibility | `TestRemoteClient_UnlockMethodNotAllowed` |

---

## 10. Future Improvements

### 10.1 Resilience Enhancements

**Retry Logic for Transient Failures**

Currently, network errors are not retried. Proposed enhancement:

```go
type RetryPolicy struct {
    MaxAttempts    int
    InitialBackoff time.Duration
    MaxBackoff     time.Duration
}

func WithRetry(policy RetryPolicy, operation func() error) error {
    for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
        err := operation()
        if err == nil || !isTransientError(err) {
            return err
        }
        
        backoff := min(
            policy.InitialBackoff * (1 << attempt),
            policy.MaxBackoff,
        )
        time.Sleep(backoff)
    }
    return err
}
```

### 10.2 Performance Improvements

**Credential Caching**

Currently, credential helpers are invoked on every request. Proposed enhancement:

```go
type CachedCredentials struct {
    credentials map[string]cachedCred
    mu          sync.RWMutex
    ttl         time.Duration
}

type cachedCred struct {
    cred      auth.Credential
    expiresAt time.Time
}
```

### 10.3 Hash Collision Mitigation

Current workspace hash uses 8 bytes (64 bits). For large-scale deployments:

```go
// Current: potential collision with many workspaces
return "ws-" + hex.EncodeToString(h[:8])   // 64-bit

// Proposed: safer for enterprise scale
return "ws-" + hex.EncodeToString(h[:16])  // 128-bit
```

### 10.4 State Versioning

Keep historical versions for audit and recovery:

```
state-production           # Current state
state-production-v1        # Version 1
state-production-v2        # Version 2
state-production-v3        # Version 3 (current)
```

Configuration:
```hcl
backend "oci" {
  repository = "registry.example.com/infrastructure/tofu-state"
  
  versioning {
    enabled      = true
    max_versions = 10
  }
}
```

### 10.5 State Encryption

Client-side encryption before pushing to registry:

```hcl
backend "oci" {
  repository = "registry.example.com/infrastructure/tofu-state"
  
  encryption {
    type   = "aes-gcm"
    key_id = "alias/opentofu-state-key"  # KMS key
  }
}
```

### 10.6 Multi-Registry Replication

Disaster recovery with automatic replication:

```hcl
backend "oci" {
  primary_repository = "primary.registry.com/tofu-state"
  
  replicas = [
    "dr-region.registry.com/tofu-state",
    "backup.registry.com/tofu-state"
  ]
}
```

### 10.7 Structured Logging

Enhanced debugging with structured logs:

```go
slog.Debug("OCI operation",
    "operation", "push_state",
    "registry", registryDomain,
    "repository", repoPath,
    "workspace", workspace,
    "size_bytes", len(data),
    "duration_ms", elapsed.Milliseconds(),
)
```

---

## 11. Security Considerations

### 11.1 Data at Rest

- State is stored as-is in the registry
- Encryption depends on registry configuration
- Consider client-side encryption for sensitive data (see §10.5)

### 11.2 Data in Transit

- TLS required by default
- Custom CA bundles supported for private PKI
- Insecure mode available but discouraged

### 11.3 Access Control

- Relies on registry's access control mechanisms
- Supports fine-grained repository permissions
- Compatible with RBAC systems (Harbor, etc.)

### 11.4 Credential Security

- Credentials never logged or stored in state
- Credential helpers preferred over static tokens
- Short-lived tokens supported (IAM roles, OIDC)

---

## 12. Compatibility Matrix

### 12.1 Tested Registries

| Registry | Version | Status | Notes |
|----------|---------|--------|-------|
| Harbor | 2.x | ✅ Full support | |
| GHCR | - | ✅ Works | DELETE returns 405, handled |
| AWS ECR | - | ✅ Full support | Use ecr-login helper |
| Azure ACR | - | ✅ Full support | |
| Google GCR | - | ✅ Full support | |
| Docker Hub | - | ✅ Full support | |
| Quay.io | - | ✅ Full support | |
| Distribution | 2.8+ | ✅ Full support | Reference implementation |

### 12.2 OpenTofu Versions

| OpenTofu Version | Backend Support |
|------------------|-----------------|
| 1.11.x | ✅ Full support |
| 1.10.x | ❌ Not available |
| 1.9.x | ❌ Not available |

---

## Appendix A: Quick Start

### A.1 Basic Configuration

```hcl
terraform {
  backend "oci" {
    repository = "ghcr.io/myorg/tofu-state"
  }
}
```

Or using environment variable:
```bash
export TF_BACKEND_OCI_REPOSITORY="ghcr.io/myorg/tofu-state"
```

```hcl
terraform {
  backend "oci" {}
}
```

### A.2 With Custom CA

```hcl
terraform {
  backend "oci" {
    repository = "harbor.internal.example.com/infrastructure/state"
    ca_file    = "/etc/ssl/certs/internal-ca.crt"
  }
}
```

### A.3 Docker Credentials Setup

```bash
# For GHCR
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# For ECR (with credential helper)
# ~/.docker/config.json:
{
  "credHelpers": {
    "123456789.dkr.ecr.us-east-1.amazonaws.com": "ecr-login"
  }
}
```

### A.4 Initialize and Use

```bash
# Initialize backend
tofu init

# Normal operations
tofu plan
tofu apply

# State is now stored in your OCI registry
```

---

## Appendix B: Troubleshooting

### B.1 Authentication Errors

**Symptom:** `unauthorized: authentication required`

**Solutions:**
1. Verify credentials: `docker login <registry>`
2. Check token permissions (read/write access)
3. Verify credential helper is installed

### B.2 TLS Errors

**Symptom:** `x509: certificate signed by unknown authority`

**Solutions:**
1. Add CA certificate: `ca_file = "/path/to/ca.crt"`
2. For testing only: `insecure = true`

### B.3 Lock Stuck

**Symptom:** Cannot acquire lock, previous process crashed

**Solution:**
```bash
tofu force-unlock <lock-id>
```

### B.4 Registry Compatibility

**Symptom:** Unexpected errors with specific registry

**Debug:**
1. Enable trace logging: `TF_LOG=TRACE tofu plan`
2. Check registry supports OCI Distribution Spec v2
3. Verify artifact type support

---

**Document Version:** 1.0  
**Implementation:** `internal/backend/remote-state/oci/`  
**Last Updated:** December 20, 2025
