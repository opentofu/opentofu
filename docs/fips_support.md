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

## Provider GPG Signature Validation (Known Limitation)

OpenTofu verifies the authenticity of provider plugins using GPG signatures. This verification process relies on the `github.com/ProtonMail/go-crypto/openpgp` library, which is not FIPS 140-3 compliant.

**When FIPS mode is enabled, GPG signature validation is automatically skipped.**

### How This Works

When OpenTofu detects that FIPS mode is active (`GODEBUG=fips140=on`):

1.  A warning is logged: `"Skipping GPG validation of provider package [name]: FIPS mode is enabled and the underlying GPG library (ProtonMail/go-crypto) is not FIPS-compliant"`
2.  GPG signature verification is bypassed
3.  Provider package integrity relies on:
    - The secure **FIPS-validated TLS connection** to the provider registry
    - Checksum verification using FIPS-approved hash algorithms

This approach maintains security through FIPS-validated TLS while avoiding the use of non-FIPS-compliant cryptographic libraries.

### Security Implications

In FIPS mode, provider authenticity is verified through:
- **✅ FIPS-validated TLS** securing the connection to the registry
- **✅ FIPS-approved hash algorithms** for checksum verification
- **❌ GPG signatures** are NOT verified (library incompatibility)

Organizations with strict FIPS requirements should evaluate whether this security model meets their compliance needs.

### Future Improvements

Future versions may explore FIPS-compliant alternatives for GPG signature verification if they become available in the Go ecosystem.