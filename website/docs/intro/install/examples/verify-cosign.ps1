$version = [version]"YOUR_OPENTOFU_VERSION"
$identity = "https://github.com/opentofu/opentofu/.github/workflows/release.yml@refs/heads/v${version.Major}.${version.Minor}"
# For alpha and beta builds use /main
cosign.exe `
    verify-blob `
    --certificate-identity $identity `
    --signature "tofu_YOURVERSION_REPLACEME.sig" `
    --certificate "tofu_YOURVERSION_REPLACEME.pem" `
    --certificate-oidc-issuer "https://token.actions.githubusercontent.com" `
    "tofu_YOURVERSION_REPLACEME_SHA256SUMS"