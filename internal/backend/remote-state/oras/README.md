# OpenTofu ORAS Remote State Backend

## Status

This backend is currently in **beta** and implements Phase 1 reliability improvements for OCI registry state storage. It should be evaluated carefully before use in mission‑critical production environments:

- ✅ **Lock Generation Semantics**: Atomic lock versioning prevents concurrent holders (lock race eliminated)
- ✅ **Stale Lock Cleanup**: When `lock_ttl > 0`, stale locks from crashed processes are automatically cleared during lock acquisition
- ⚠️ **Async State Retention**: Version cleanup runs asynchronously to avoid blocking `terraform apply`

OpenTofu/Terraform Core has not added new remote state backends in years, preferring the generic HTTP backend for custom implementations. This package serves as a reference implementation and starting point for those who want native OCI registry state storage.

## The Big Idea

Use an OCI registry as the durable store for OpenTofu state.

Each workspace's state is written as an OCI manifest plus layer inside a registry you control (for example the registry you already use for container images). Lock information is published as a separate manifest/tag. This lets you reuse existing registry authentication, authorization, and operational tooling.

> **Note**: The implementation has primarily been exercised against GitHub Container Registry (`ghcr.io`). Other registries _should_ work but haven't yet been validated.

## Security Considerations

State snapshots frequently contain sensitive data (provider credentials, resource attributes, and sometimes plaintext secrets). Treat the OCI repository with the same care as any other credentials store:

- **Enable client-side encryption**: OpenTofu supports [state encryption](https://opentofu.org/docs/language/state/encryption/) which encrypts state before it leaves your machine. This is the recommended approach when storing state in any remote backend, including OCI registries.
- Keep the repository private and issue least-privilege tokens.
- Prefer registries that provide encryption at rest and audit trails.
- Be deliberate about retention/versioning; a long history increases exposure.
- Evaluate whether your threat model allows state to live in a container registry at all.

## Parameters

All configuration lives in the backend block. Some options also respond to environment variables.

- `repository` (required): OCI repository in the form `<registry>/<repository>`, without tag or digest.
  - Env: `TF_BACKEND_ORAS_REPOSITORY`
- `compression` (optional, default `none`): `none` or `gzip`.
- `insecure` (optional, default `false`): disable TLS verification.
- `ca_file` (optional): PEM bundle of additional CAs to trust.

Retry/backoff:
- `retry_max` (optional, default `2`): retries for transient registry requests. Total attempts = `retry_max + 1`.
  - Env: `TF_BACKEND_ORAS_RETRY_MAX`
- `retry_wait_min` (optional, default `1`): minimum backoff in seconds.
  - Env: `TF_BACKEND_ORAS_RETRY_WAIT_MIN`
- `retry_wait_max` (optional, default `30`): maximum backoff in seconds.
  - Env: `TF_BACKEND_ORAS_RETRY_WAIT_MAX`

Locking:
- `lock_ttl` (optional, default `0`): lock TTL in seconds. When greater than 0, stale locks older than this are automatically cleared during lock acquisition. Set to `0` to disable.
  - Env: `TF_BACKEND_ORAS_LOCK_TTL`

Rate limiting:
- `rate_limit` (optional, default `0`): maximum registry requests per second. `0` disables.
  - Env: `TF_BACKEND_ORAS_RATE_LIMIT`
- `rate_limit_burst` (optional, default `0`): maximum burst size. If `rate_limit > 0` and burst is `0`, we fall back to burst `1`.
  - Env: `TF_BACKEND_ORAS_RATE_LIMIT_BURST`

Versioning:
- `max_versions` (optional, default `0`): maximum historical versions to retain. `0` disables versioning, `>0` enables versioning with that retention limit. Cleanup always runs asynchronously.

## How State Is Stored (Tags)

Tags act as stable references:

- State: `state-<workspaceTag>`
- Lock: `locked-<workspaceTag>`

`workspaceTag` equals the workspace name if it is a valid OCI tag. Otherwise the backend uses a stable `ws-<hash>` form and persists the real workspace name in OCI annotations.

If versioning is enabled (`max_versions > 0`), each successful state write also tags the manifest as:

- `state-<workspaceTag>-v<integer>`

Version numbers are computed by scanning existing tags and picking `(max + 1)`.

## Under the Hood (Wire Format)

State objects use ORAS manifest v1.1 packing.

- Manifest `artifactType`: `application/vnd.terraform.state.v1`
- Layer media type:
  - `application/vnd.terraform.statefile.v1` (no compression)
  - `application/vnd.terraform.statefile.v1+gzip` (gzip)
- Annotations:
  - `org.terraform.workspace`: workspace name
  - `org.terraform.state.updated_at`: RFC3339 timestamp (changes on every write)

Lock objects:
- Manifest `artifactType`: `application/vnd.terraform.lock.v1`
- Annotations:
  - `org.terraform.workspace`: workspace name
  - `org.terraform.lock.id`: lock ID
  - `org.terraform.lock.info`: JSON-encoded lock metadata

Reads are strict: unexpected artifact types or media types raise errors instead of silently proceeding.

## Locking

The lock lives at `locked-*` tags and carries metadata in annotations. Unlocking deletes that manifest when possible. Some registries do not support manifest deletion via OCI `DELETE`. When deletion fails with HTTP 405 the backend retags the lock reference to an `unlocked-*` placeholder instead.

### Phase 1 Lock Improvements (Generation Semantics)

**Problem**: In the original implementation, two concurrent processes could both pass the initial lock check, write their lock manifests, and both believe they held the lock (the last writer wins, but the loser doesn't realize it lost).

**Solution**: Lock manifests now include atomic **generation numbers** stored in the `org.terraform.lock.generation` annotation as JSON:

```json
{
  "generation": 42,
  "lease_expiry": 1234567890,
  "holder_id": "process-abc"
}
```

When acquiring a lock:
1. Read current generation FIRST (before any modifications)
2. Check if lock is stale and clear it if needed
3. Increment generation: `newGen = currentGen + 1`
4. Write lock manifest with `newGen`
5. **Post-write verification**: Re-read the lock and verify `generation == newGen`
   - If mismatch: Another process won the race → return LockError
   - If match: We hold the lock

**Why this works for stale lock cleanup**: Reading the generation BEFORE clearing a stale lock ensures that if another process races to acquire during the cleanup window, the processes will write different generation numbers. The post-write verification will detect the conflict because only one generation can win.

### Stale Lock Cleanup (TTL & Background)

**Scenario 1: Lock Timeout During Acquisition**

If `lock_ttl` is configured (e.g., `lock_ttl = 300` for 5 minutes), when a process attempts to acquire a lock:
1. Check if existing lock is stale (created more than `lock_ttl` ago)
2. If stale: automatically clear it before attempting acquisition
3. Continue with normal generation-based lock acquisition

This handles the common case where a crashed process left a lock behind. The next `terraform apply` automatically clears it without manual intervention.

**Lease Expiry Metadata**

When `lock_ttl > 0`, each lock includes `lease_expiry` in nanoseconds since Unix epoch. This allows external tools to inspect lock expiry without reading lock creation timestamps.

## Versioning & Retention

When `max_versions > 0`, versioning is enabled and every successful state write gets an additional `-vN` tag. Older versions beyond the limit are pruned automatically using asynchronous (non-blocking) cleanup:

**Async Retention**

Cleanup queues to a background goroutine with 30-second timeout:
- **Pros**: `terraform apply` returns immediately without waiting for cleanup
- **Cons**: Old versions may persist briefly until cleanup completes (non-fatal)
- **Behavior**: Always runs in the background, providing optimal performance

```hcl
max_versions = 10  # 0 = disabled, >0 = enabled with retention
```

**GHCR Deletion Fallback**

Registries such as `ghcr.io` frequently return HTTP 405 for OCI `DELETE`. In that case the backend falls back to deleting the corresponding package version using the GitHub Packages API. The token used for registry access must therefore include `delete:packages` when retention is enabled. If deletion is impossible, stale versions may remain but cleanup is logged and ignored (non-blocking).

## Authentication

Credentials are discovered in this order:

1. Docker credential helpers (Docker config / credential store).
2. OpenTofu CLI host credentials (`tofu login`).

If Docker credentials are missing or fail, the backend falls back to CLI tokens.

## Usage

Example (minimal):

```hcl
terraform {
  backend "oras" {
    repository = "ghcr.io/acme/opentofu-state"
  }
}
```

Example (production-ready with Phase 1 improvements):

```hcl
terraform {
  backend "oras" {
    repository  = "ghcr.io/myorg/infra-state"
    compression = "gzip"
    
    # Lock reliability
    lock_ttl = 300   # 5 minutes

    # State versioning
    max_versions = 10  # 0 = disabled, >0 = enabled with retention
  }

  # Client-side state encryption (OpenTofu feature)
  encryption {
    key_provider "pbkdf2" "main" {
      passphrase = var.state_passphrase  # or use TF_ENCRYPTION env var
    }

    method "aes_gcm" "main" {
      key_provider = key_provider.pbkdf2.main
    }

    state {
      method = method.aes_gcm.main
    }

    plan {
      method = method.aes_gcm.main
    }
  }
}

variable "state_passphrase" {
  type      = string
  sensitive = true
}
```

> **Note**: For production, consider using a KMS-backed key provider (AWS KMS, GCP KMS, etc.) instead of PBKDF2 with a passphrase.

Example (GitHub Actions CI):

```yaml
- name: ToFu Init
  env:
    TF_BACKEND_ORAS_REPOSITORY: ghcr.io/${{ github.repository_owner }}/tf-state
  run: |
    echo "${{ secrets.GITHUB_TOKEN }}" | docker login ghcr.io -u ${{ github.actor }} --password-stdin
    tofu init
```

The token used for `docker login` must be able to read/write the repository. If you enable version retention or rely on GHCR deletion fallbacks, the token must also be permitted to enumerate and delete package versions (`delete:packages`).

## Troubleshooting

**"unauthorized" or "denied" errors**
- Confirm `docker login <registry>` works with the same credentials.
- For GHCR ensure the token grants `read:packages` + `write:packages` (and `delete:packages` if retention is on).

**Lock stuck after crashed run (Phase 1 solutions)**

Option 1: **Automatic cleanup with TTL** (recommended)
```hcl
lock_ttl = 300  # 5 minutes
```
When `lock_ttl` is set, stale locks are automatically detected and cleared during lock acquisition. The next `terraform apply` will clear any lock older than the TTL.

Option 2: **Manual cleanup**
- Delete the `locked-<workspace>` tag from the registry UI or API

**Slow applies due to retention cleanup**

Version cleanup always runs asynchronously in the background:
```hcl
max_versions = 10
```
Cleanup happens in the background without blocking `terraform apply`.

**Version deletion on GHCR**
- GHCR does not support OCI `DELETE`; the backend automatically falls back to the GitHub Packages API.
- Ensure the token has `delete:packages` permission for this fallback to work.
- If the API call also fails (e.g., insufficient permissions), old versions accumulate but writes still succeed (cleanup is logged and ignored).

**Debug mode**

```bash
TF_LOG=DEBUG tofu plan
```

## Testing

Unit tests run offline with an in-memory fake OCI registry:

```bash
go test ./internal/backend/remote-state/oras
```

## Limitations / Future Enhancements

### Phase 1 Improvements (Completed ✅)

- ✅ **Lock race condition eliminated**: Generation semantics ensure atomic lock acquisition
- ✅ **Stale lock cleanup**: Both on-demand (during acquisition) and proactive (background) modes
- ✅ **Non-blocking retention**: Async mode always used, preventing slow applies in CI/CD pipelines

### Remaining Limitations

- Only GHCR is exercised in automated tests; other registries may exhibit different quirks.
- Registries that refuse OCI `DELETE` degrade lock/unlock behavior (GHCR has a dedicated fallback via GitHub Packages API).
- `insecure = true` disables TLS verification—use only in controlled environments.

### Future Enhancements (Phase 2+)

- Better visibility into lock/version tags (list commands, tooling).
- Multi-registry support with capability detection.
- Registry-specific retention strategies beyond GHCR.
- Validation with other registries such as ECR, GCR, ACR, Harbor, etc.
- Eliminate GitHub API dependency for GHCR deletion.
