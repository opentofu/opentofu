#!/bin/sh
set -e

TOFU_VERSION="1.6.0-alpha2"

OS="$(uname | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m | sed -e 's/aarch64/arm64/' -e 's/x86_64/amd64/')"
TEMPDIR="$(mktemp -d)"
trap 'rm -rf "${TEMPDIR}"' EXIT # Cleanup on exit

echo "Downloading OpenTofu..."
wget "https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_${OS}_${ARCH}.zip" -O "${TEMPDIR}/tofu.zip" || {
  echo "Error downloading OpenTofu. Please check the provided version and your internet connection." >&2
  exit 1
}

echo "Extracting OpenTofu..."
unzip "${TEMPDIR}/tofu.zip" -d "${TEMPDIR}/tofu" || {
  echo "Error unzipping OpenTofu. The downloaded file may be corrupted." >&2
  exit 1
}

echo "Installing OpenTofu..."
sudo mv "${TEMPDIR}/tofu/tofu" /usr/local/bin/tofu || {
  echo "Error installing OpenTofu. Please ensure you have sufficient permissions." >&2
  exit 1
}

echo "OpenTofu is now available at /usr/local/bin/tofu."