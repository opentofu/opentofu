# FIPS 140-3 Support in OpenTofu (Experimental)

OpenTofu can be operated in a mode that facilitates FIPS 140-3 compliance by leveraging the native FIPS support built into Go (version 1.24 and later). This feature is currently considered **experimental**.

## Enabling FIPS Mode

FIPS mode is enabled at runtime by setting the `GODEBUG` environment variable when executing OpenTofu commands:

```sh
export GODEBUG=fips140=on
tofu plan
tofu apply
# etc.
```

Alternatively, `GODEBUG=fips140=only` can be used, which will cause non-FIPS-compliant cryptographic operations to panic or return errors, providing a stricter (but potentially less compatible) mode.

**Requirements:**

*   **Go Version:** OpenTofu must be built with Go 1.24 or later.
*   **Supported Platforms:** Go's native FIPS mode is supported on most common platforms (Linux, macOS, Windows - 64-bit only). It is **not** supported on OpenBSD, Wasm, AIX, or 32-bit Windows.

## Implications of FIPS Mode

When `GODEBUG=fips140=on` is set:

*   **Validated Cryptography:** Cryptographic operations handled by Go's standard library (like AES-GCM used for state encryption, SHA hashes, and TLS) will utilize the FIPS-validated cryptographic module.
*   **Self-Tests:** The Go runtime performs integrity and known-answer self-tests for cryptographic algorithms at startup or first use.
*   **TLS Restrictions:** The `crypto/tls` package enforces FIPS-compliant settings. It will refuse to negotiate non-compliant TLS versions, cipher suites, signature algorithms, or key exchange mechanisms. This affects provider plugin communication (mTLS), backend communication (e.g., S3, Terraform Cloud/Enterprise), module registry access, and the `tofu login` process.
*   **Random Number Generation:** `crypto/rand` uses a FIPS-approved DRBG (Deterministic Random Bit Generator).
*   **Performance:** Some cryptographic operations, particularly key generation, may experience a performance impact due to required FIPS self-tests (e.g., pairwise consistency tests).

## Provider GPG Signature Validation (Potential Limitation)

OpenTofu verifies the authenticity of provider plugins using GPG signatures. This verification process currently relies on the `github.com/ProtonMail/go-crypto/openpgp` library.

**The compatibility of this library with Go's FIPS mode is currently unverified.**

It is possible that the `go-crypto/openpgp` library uses cryptographic algorithms or implementations that conflict with FIPS requirements. If this is the case, running `tofu init` in FIPS mode might fail during provider signature validation.

**Current Plan:**

1.  **Testing:** Compatibility will be tested thoroughly (Phase 2 of the implementation plan).
2.  **Mitigation (If Necessary):** If incompatibility is confirmed, the plan is to investigate replacing the `go-crypto/openpgp` library with a FIPS-compatible alternative (Phase 3). If replacement is not feasible, a decision will be made whether to conditionally disable GPG validation in FIPS mode (with clear documentation) or block FIPS support.

Users relying on FIPS compliance should be aware of this potential limitation during the experimental phase. The status will be updated as testing progresses.