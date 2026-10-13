# oracle-oci backend implementation and validation record

The `oracle-oci` backend uses the official OCI Go SDK and OpenTofu's backend and remote state interfaces. User configuration, the authentication support matrix, examples, IAM policies, and migration instructions are in [the backend documentation](../../../../website/docs/language/settings/backends/oracle-oci.mdx).

`oracle-oci` is the canonical backend name. The earlier `oci` name is accepted as a compatibility alias and normalized to `oracle-oci`; it uses the same state and locking implementation. The Go package remains `oci`, and OCI provider names and `OCI_*` environment variables are unchanged.

## Scope and provenance

The repository's `CONTRIBUTING.md`, `AGENTS.md`, and `contributing/DEVELOPING.md` were read before implementation. OpenTofu prohibits opening pull requests with LLM-generated or assisted contributions. This work is a local implementation requested by the user; it must not be submitted as an OpenTofu pull request. No pull request or commit is part of this work.

The existing repository had no OCI backend, OCI SDK dependency, or OCI backend documentation. Architecture references were OpenTofu's `backend.Backend`, `remote.Client`, `remote.ClientLocker`, `remote.State`, and `statemgr.Locker` interfaces, the backend registration table, and the existing backend tests. Authentication is constructed before provider initialization and never reads provider configuration.

The [OCI provider repository](https://github.com/oracle/terraform-provider-oci/) was examined at commit `ad4dcb798cc2a64eb77285b894f7d6edd73e176b`, using these sources:

- `internal/provider/provider.go` and `provider_framework.go`: both schemas, environment defaults, mode dispatch, profile handling, and credential composition.
- `internal/provider/provider_test.go`: authentication, environment, and precedence tests.
- `internal/provider/workload_identity_federation.go` and `workload_identity_federation_test.go`: the full supported authentication list, both token exchange authentication methods, required fields, defaults, and projected-token behavior.
- `internal/globalvar` and `internal/utils`: mode constants, environment names, profile checks, and certificate filenames.
- The provider's vendored OCI Go SDK `v65.126.2`: config file resolution, identity provider requirements, signer refresh behavior, and token exchange signing-key behavior.

The downloaded SDK module records tag `v65.126.2` at commit `b9f1e5b7b6f47217a5322d0de9c8f480d6c237cf` in its Go module origin metadata.

The supported modes are `ApiKey`, `InstancePrincipal`, `InstancePrincipalWithCerts`, `SecurityToken`, `ResourcePrincipal`, `OKEWorkloadIdentity`, and `WorkloadIdentityFederation`. Workload identity federation supports `OAuthClientCredentials` and `InstancePrincipal` exchange authentication. The user guide records backend initialization extensions and differences from the examined provider.

The inspection also included the provider's legacy environment-only tenancy override, certificate directory lookup, and on-behalf-of delegation. Their lowercase legacy environment precedence is preserved separately from ordinary uppercase schema-variable defaults. Delegation supplements the seven modes and is not another enum value. Missing enabled delegation credentials fail explicitly rather than falling back to the base identity. The backend retains secure SDK transport behavior and omits the provider's transport/testing options that replace certificate pools or disable TLS verification.

No HashiCorp Terraform OCI backend source, patch, or documentation was used. Backend code and documentation were written independently; the OCI provider was inspected for behavioral requirements rather than copied into OpenTofu. The SDK is an imported dependency. Its [dual license](https://github.com/oracle/oci-go-sdk/blob/v65.126.2/LICENSE.txt) permits UPL-1.0 or Apache-2.0; Apache-2.0 is selected because it is approved by this repository's `.licensei.toml`. The OCI provider reference is MPL-2.0. Newly written Go files carry the repository's MPL-2.0 header; imported dependencies retain their own copyright and license notices.

The new direct SDK dependency and transitive dependencies have these licenses, checked from the downloaded modules' license files:

| Dependency | Version | Selected license |
| --- | --- | --- |
| `github.com/oracle/oci-go-sdk/v65` | `v65.126.2` | Apache-2.0, selected from the dual license |
| `github.com/gofrs/flock` | `v0.10.0` | BSD-3-Clause |
| `github.com/sony/gobreaker/v2` | `v2.4.0` | MIT |
| `github.com/youmark/pkcs8` | `v0.0.0-20240726163527-a2c0da244d78` | MIT |

These licenses appear in the repository's approved list. Redistribution must preserve applicable dependency notices; this inspection is separate from the automated repository license check.

## Slack requirements consulted

The requested [Slack conversation](https://oracle.enterprise.slack.com/archives/C0C5NLBCB6D) was accessible. Its available history covered September 30 through October 8, 2026. The conversation requested a native backend with the normal OpenTofu customer experience and an independent implementation rather than reuse of a HashiCorp contribution:

- [Native backend experience](https://oracle.enterprise.slack.com/archives/C0C5NLBCB6D/p1791483047084419).
- [Independent implementation without using the HashiCorp work](https://oracle.enterprise.slack.com/archives/C0C5NLBCB6D/p1791483530684349).
- [No forking](https://oracle.enterprise.slack.com/archives/C0C5NLBCB6D/p1791483293587469).
- [Build a backend to assess it](https://oracle.enterprise.slack.com/archives/C0C5NLBCB6D/p1791483614829639).

That conversation did not add specific authentication, IAM, locking, migration, or retry requirements. Attached images and linked proxy-design documents were not used as implementation specifications. Slack comments about legal acceptability do not override OpenTofu's repository contribution policy.

## Validation categories

| Category | What it can establish | What it cannot establish |
| --- | --- | --- |
| Local credential fixtures | PEM/certificate validation, named profiles, credential precedence, JWT expiration checks, token-file rotation, and sensitive-error redaction | Whether OCI accepts the identity or its IAM policies |
| Injected identity factories | Dispatch and inputs for host-dependent instance, resource, OKE, certificate, and workload federation providers | Metadata reachability, real token exchange, automatic SDK token renewal, or runtime integration |
| Mocked Object Storage client | State reads/writes/deletion, checksums, workspaces, conditional requests, locking conflicts, retry reconciliation, and cancellation | Real Object Storage conditional request guarantees or tenancy policy behavior |
| SDK transport test | SDK signing and request/response conversion through an in-memory HTTP transport | Real OCI authentication or service acceptance |
| Live OCI validation | Real identities, bucket access, policy enforcement, token renewal, and service behavior | Coverage of other tenancies, realms, or runtimes not exercised |

Authentication test coverage is deliberately explicit:

| Mode | Completed local coverage | Runtime behavior not validated live |
| --- | --- | --- |
| `ApiKey` | Official SDK RSA/PEM handling, encrypted key and wrong passphrase, inline/path/profile precedence, named profile/default fallback, configuration file selection, and missing credentials | OCI user/key acceptance and policy enforcement |
| `InstancePrincipal` | Injected SDK factory selection, region requirement, constructor error handling, and redaction | Actual metadata service, instance federation, and renewal |
| `InstancePrincipalWithCerts` | Official SDK supplied-certificate construction, tenancy extraction, validity/matching-key checks, encrypted key, missing certificate files, and injected factory selection | OCI certificate acceptance, federation service, and certificate rotation |
| `SecurityToken` | Official SDK session profile and key handling, session-profile key precedence and raw/default fallback, token-file rereading, rotated token, expired/malformed/missing token, and protected errors | Real CLI session acceptance and external session renewal |
| `ResourcePrincipal` | Injected runtime selection, official SDK version `2.2` file-backed key/token refresh and expired/malformed token handling, version `3.0` generic construction/leaf claims with explicit region override, and missing runtime credentials | Real resource runtime, version `1.1`/`3.0` delegated trust exchanges, and OCI service renewal |
| `OKEWorkloadIdentity` | Injected SDK factory selection, region override, missing runtime credentials, constructor failure, and redaction | OKE proxymux, mounted service account integration, and token renewal |
| `WorkloadIdentityFederation` | Required fields/defaults, both exchange authentication methods through factories, official SDK OAuth exchange through a mocked transport, projected-token rotation, expired/missing JWT, opaque subject inputs, and protected errors | Identity-domain trust, real RPST/UPST exchange, instance-principal signed exchange, and service renewal |

Legacy tenancy and certificate environment precedence, delegation token source precedence, and request signing are tested separately from authentication-mode dispatch. The delegated request tests use an in-memory SDK transport and rotating token files; they do not prove OCI accepts a delegated token.

The Object Storage client tests include state lifecycle/checksums, simultaneous initial creation, stale write/delete rejection, absent-state baselines, upload/delete outcome reconciliation, replacement protection, missing ETag handling, bounded retries, expired-credential and IAM errors, lock conflicts, mismatched IDs, stolen-lock checks, unlock replacement races, malformed locks, cancellation, and official SDK signing/serialization through an in-memory HTTP transport. Backend tests cover pure configuration preparation, independent initialization, workspace paths and paginated listing, initial-state locking/rechecking, lineage preservation, and deletion behavior.

Signing tests force a token/key renewal between credential reads and verify the official SDK's RSA signature with the renewed key, rejecting the old key. The signing boundary snapshots key ID, key, and key ID again and retries a bounded number of times until the pair is stable. Each service operation receives a fresh SDK client value and signing snapshot so that a canceled operation's SDK retry goroutine cannot replace another operation's paired credentials. A regression test deliberately overlaps an old canceled operation with a new renewed identity and cryptographically verifies both signatures. Normal service operations are also serialized. These are deterministic renewal and cancellation regression tests, not live runtime renewal validation.

## Live validation requirements

Live validation requires credentials explicitly authorized for a disposable, pre-existing bucket. Use a unique object key and workspace prefix. Do not run live state-write tests against a production state bucket. The selected principal needs only the object permissions described in the user guide; bucket administration is separate.

`TestAccOCIObjectStorage` is opt-in. Set authentication environment variables for the mode being checked, then run:

```shell
TF_ACC=1 TF_OCI_NAMESPACE=example_namespace TF_OCI_BUCKET=disposable-test-bucket \
  go test ./internal/backend/remote-state/oci -run TestAccOCIObjectStorage -count=1
```

The test creates a UUID-scoped prefix, exercises state and locking with the official SDK, checks a named workspace, and cleans up its objects. It never creates or deletes the bucket. A normal package test run skips this test. The same live test must be run separately within each identity runtime to establish live authentication coverage.

For each authentication mode, exercise backend initialization, state read/write/delete, named workspace creation/listing/deletion, concurrent lock acquisition, mismatched unlock, and stale state writes. Observe renewal over a token lifetime for each refreshable identity. Test missing and expired credentials and verify that errors and enabled OpenTofu logs do not expose tokens, private keys, or state bodies.

Validate instance metadata, instance certificates and their rotation, every resource principal runtime/version used in deployment, OKE projected token rotation, both workload exchange authentication methods, both RPST and UPST exchanges when enabled, Vault permissions, versioned-object deletion, and recovery after a lost HTTP response. Test migration from each deployed source backend using protected state backups.

OCI has no transaction spanning the state and lock objects. A force-unlock must only occur after the old writer has stopped. Tests for ownership checks and state ETag preconditions cannot establish that forcibly unlocking an active writer is safe.

## Results and remaining limitations

No live OCI validation has been performed for this implementation. Unit-test results do not imply that any OCI authentication runtime, identity-domain trust, or IAM policy was exercised. Generic workload federation authorization depends on the tenancy's trust mapping and requires a deployment-specific principal policy; the public JWT-to-RPST documentation does not provide a universal exchange IAM permission statement.

SDK `v65.126.2` uses a custom signing path for Object Storage `PutObject` that bypasses its HTTP 401 automatic re-signing retry. Token renewal during normal signing remains available; a returned upload HTTP 401 is reported without a backend authentication retry. State bodies remain seekable for SDK request replay where it applies. Object uploads are single-part, and the backend exposes Vault KMS selection but no customer-supplied SSE-C key input. SDK custom CA and mutual TLS controls are inherited and documented, but actual TLS handshakes and certificate-refresh behavior have not been validated against OCI.

Checks recorded on October 13, 2026:

| Check | Result |
| --- | --- |
| Final OCI/backend registration race and coverage run | Passed: 67 local OCI top-level tests, one live test skipped; OCI coverage 88.7% (6.409 seconds), backend registration coverage 27.1% (2.935 seconds) |
| Latest targeted signing race regression suite | Passed: five signing tests, including overlapping canceled SDK signing and renewed credentials |
| Final native OpenTofu build | Passed; `cmd/tofu` built successfully |
| CLI initialization smoke check | OCI backend recognized; an unsupported authentication mode was rejected during initialization without provider configuration or an OCI network request |
| Final `go vet` for OCI backend and backend registration | Passed |
| `go mod verify` | Passed: all modules verified |
| Module tidy, Go formatting, and `git diff --check` | Passed |
| User-guide MDX lint and external source links | Passed: zero errors and warnings |
| HCL documentation examples | All nine backend examples parsed successfully |
| Local documentation links and sidebar registration | Passed |
| Direct/transitive new dependency license review | Passed against the repository's approved list; SDK Apache-2.0 selected, three transitive licenses reviewed |
| Automated repository license check | Not run |
| Full repository test suite | Not run |
| Live OCI tests | Not run; opt-in test skipped |

The final checks used Go `1.27.1` on Darwin ARM64. The race/coverage test enabled `OCI_GO_SDK_DEBUG=v`, left `OCI_GO_SDK_LOG_OUTPUT_MODE` empty, and set `TF_ACC=0`. SDK verbose logging remained suppressed by the backend and sensitive-error tests passed. These reproduction commands run the local checks without opting into OCI access:

```shell
OCI_GO_SDK_DEBUG=v OCI_GO_SDK_LOG_OUTPUT_MODE= TF_ACC=0 \
  go test -race -count=1 -coverprofile /tmp/oci-backend.coverage \
  ./internal/backend/remote-state/oci ./internal/backend/init
go build -o /tmp/oci-backend-tofu ./cmd/tofu
go vet ./internal/backend/remote-state/oci ./internal/backend/init
go mod verify
go mod tidy -diff
git diff --check
```

The documentation checker passed with the repository's existing MDX lint configuration and external source-link validation. All nine HCL examples were also parsed using OpenTofu's HCL parser. Neither the successful local checks nor coverage measurements imply live OCI authentication, network, or IAM validation.

The subsequent naming change was checked with `go test ./internal/backend/init ./internal/backend/remote-state/oci -run 'TestInit_|TestPrepareConfig|TestWorkspaceObjectPaths' -count=1`. Both package checks passed (backend registration 2.523 seconds, OCI backend 1.702 seconds), including verification that both `oracle-oci` and the compatibility alias `oci` resolve to canonical `oracle-oci`. The nine renamed HCL examples, documentation links, and both sidebar entries were also checked. The full race/coverage results above remain the implementation validation record; authentication behavior was unchanged by the naming edit.
