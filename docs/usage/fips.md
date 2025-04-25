# Using OpenTofu in FIPS Mode (Experimental)

OpenTofu can be run in a mode that utilizes FIPS 140-3 validated cryptographic modules provided by the underlying Go runtime. This ensures that only approved cryptographic algorithms are used for operations like TLS connections and potentially state file encryption (depending on the backend configuration).

**Note:** This feature is currently **experimental**.

## Enabling FIPS Mode

To enable FIPS mode, you must:

1.  **Use Go 1.24 or later:** FIPS support relies on features introduced in Go 1.24. Ensure your OpenTofu binary was compiled with Go 1.24+.
2.  **Set the `GODEBUG` environment variable:** Before running any `tofu` command, set the environment variable `GODEBUG=fips140=on`.

   ```shell
   export GODEBUG=fips140=on
   tofu plan
   ```

## Implications of FIPS Mode

When FIPS mode is enabled:

*   **Stricter Cryptography:** Only FIPS-approved cryptographic algorithms will be permitted. This primarily affects TLS connections (e.g., to provider registries, backend storage). Connections requiring older or non-compliant algorithms may fail.
*   **Built-in Self-Tests:** The Go runtime performs self-tests on startup to ensure the integrity of the cryptographic modules.
*   **Potential Performance Impact:** While generally minimal, there might be a slight performance overhead due to the stricter cryptographic requirements and self-tests.

## Known Limitations

*   **GPG Provider Signature Validation Skipped:** Due to limitations in the underlying OpenPGP library used by OpenTofu, **GPG signature validation for provider packages is automatically skipped when FIPS mode is enabled.** OpenTofu will log a warning when this occurs. In this scenario, the integrity of the provider package relies solely on the secure TLS connection to the provider registry. This limitation may be addressed in future releases if a FIPS-compliant OpenPGP library becomes available for Go.

## Supported Platforms

Go's FIPS support is available on most common platforms where OpenTofu runs, but consult the official Go documentation for the most up-to-date list. It is generally *not* available on platforms like OpenBSD, WebAssembly (Wasm), AIX, or 32-bit Windows.