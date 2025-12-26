# OpenTofu ORAS Remote State Backend

## Status

This backend is an experiment/reference implementation for storing OpenTofu state in an OCI registry via [ORAS](https://oras.land/).

Terraform Core historically avoids new remote state backends upstream. If these changes are not accepted, the intent is for this package to remain as documentation and a starting point for downstream forks.

## The Big Idea

Use an OCI registry as the durable store for OpenTofu state.

Each workspace's state is written as an OCI manifest plus layer inside a registry you control (for example the registry you already use for container images). Lock information is published as a separate manifest/tag. This lets you reuse existing registry authentication, authorization, and operational tooling.

> **Note**: The implementation has primarily been exercised against GitHub Container Registry (`ghcr.io`). Other registries _should_ work but haven't yet been validated.

## Security Considerations

State snapshots frequently contain sensitive data (provider credentials, resource attributes, and sometimes plaintext secrets). Treat the OCI repository with the same care as any other credentials store:

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
- `lock_ttl` (optional, default `0`): lock TTL in seconds. If non-zero, stale locks older than this may be cleared while acquiring a lock. `0` disables.
  - Env: `TF_BACKEND_ORAS_LOCK_TTL`

Rate limiting:
- `rate_limit` (optional, default `0`): maximum registry requests per second. `0` disables.
  - Env: `TF_BACKEND_ORAS_RATE_LIMIT`
- `rate_limit_burst` (optional, default `0`): maximum burst size. If `rate_limit > 0` and burst is `0`, we fall back to burst `1`.
  - Env: `TF_BACKEND_ORAS_RATE_LIMIT_BURST`

Versioning:
- `versioning { ... }` (optional block): turn on version tags for state snapshots.
  - `enabled` (optional, default `false`)
  - `max_versions` (optional): maximum historical versions to retain. `0` means unlimited.

## How State Is Stored (Tags)

Tags act as stable references:

- State: `state-<workspaceTag>`
- Lock: `locked-<workspaceTag>`

`workspaceTag` equals the workspace name if it is a valid OCI tag. Otherwise the backend uses a stable `ws-<hash>` form and persists the real workspace name in OCI annotations.

If versioning is enabled, each successful state write also tags the manifest as:

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

If `lock_ttl` is configured, the backend may clear a stale lock older than the TTL when acquiring a new lock.

**Note**: There is still a theoretical race where two concurrent runs both believe they acquired the lock. Combine this backend with CI concurrency controls when possible.

## Versioning & Retention

When `versioning.enabled` is true, every successful state write gets an additional `-vN` tag.

If `versioning.max_versions > 0`, older versions are pruned during new writes. This is an inline operation, not a background job.

Registries such as `ghcr.io` frequently return HTTP 405 for OCI `DELETE`. In that case the backend falls back to deleting the corresponding package version using the GitHub Packages API. The token used for registry access must therefore include `delete:packages` when retention is enabled. If deletion is impossible, stale versions may remain and the write can fail.

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

Example (gzip + versioning):

```hcl
terraform {
  backend "oras" {
    repository  = "ghcr.io/myorg/infra-state"
    compression = "gzip"

    versioning {
      enabled      = true
      max_versions = 10
    }
  }
}
```

Example (GitHub Actions CI):

```yaml
- name: ToFu Init
  env:
    TF_BACKEND_ORAS_REPOSITORY: ghcr.io/${{ github.repository_owner }}/tf-state
  run: |
    echo "${{ secrets.GITHUB_TOKEN }}" | docker login ghcr.io -u $ --password-stdin
    tofu init
```

The token used for `docker login` must be able to read/write the repository. If you enable version retention or rely on GHCR deletion fallbacks, the token must also be permitted to enumerate and delete package versions (`delete:packages`).

## Troubleshooting

**"unauthorized" or "denied" errors**
- Confirm `docker login <registry>` works with the same credentials.
- For GHCR ensure the token grants `read:packages` + `write:packages` (and `delete:packages` if retention is on).

**Lock stuck after crashed run**
- Set `lock_ttl = 300` to auto-expire locks after five minutes.
- Or manually delete the `locked-<workspace>` tag from the registry UI.

**Version deletion fails on GHCR (405 error)**
- GHCR does not support OCI `DELETE`; the backend invokes the GitHub Packages API instead.
- Ensure the token is allowed to delete package versions (`delete:packages`).
- If deletion still fails, old versions accumulate (clutter but not blocking).

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

Limitations:
- Only GHCR is exercised in automated tests; other registries may exhibit different quirks.
- Registries that refuse OCI `DELETE` degrade lock/unlock and retention behavior.
- Retention is enforced inline; there is no background compaction.
- `lock_ttl` is evaluated during lock attempts, not proactively.
- `insecure = true` disables TLS verification—use only in controlled environments.

Future enhancements:
- Better visibility into lock/version tags (list commands, tooling).
- Registry-specific retention strategies beyond GHCR.
- Validation with other registries such as ECR, GCR, ACR, Harbor, etc.
