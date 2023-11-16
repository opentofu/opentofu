#!/bin/sh
set -e
TOFU_VERSION="1.6.0-alpha2"
OS="$(uname | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m | sed -e 's/aarch64/arm64/' -e 's/x86_64/amd64/')"
TEMPDIR="$(mktemp -d)"
pushd "${TEMPDIR}" >/dev/null
wget "https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_${OS}_${ARCH}.zip"
unzip "tofu_${TOFU_VERSION}_${OS}_${ARCH}.zip"
sudo mv tofu /usr/local/bin/tofu
popd >/dev/null
rm -rf "${TEMPDIR}"
echo "OpenTofu is now available at /usr/local/bin/tofu."