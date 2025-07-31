OPENTOFU_VERSION_MAJORMINOR="Add your OpenTofu major and minor version here"
IDENTITY="https://github.com/opentofu/opentofu/.github/workflows/release.yml@refs/heads/v${OPENTOFU_VERSION_MAJORMINOR}"
# For alpha and beta builds use /main
cosign \
    verify-blob \
    --certificate-identity "${IDENTITY}" \
    --signature tofu_*.sig \
    --certificate tofu_*.pem \
    --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
    tofu_*_SHA256SUMS
