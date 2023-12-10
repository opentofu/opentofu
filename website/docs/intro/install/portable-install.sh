#!/bin/sh
set -e
# compat. for multiple shells
# Initialize the directory stack as an empty string
DIR_STACK=""

# Usage:
# pushd /path/to/new/directory
# popd

pushd() {
    # Add the current directory to the stack
    DIR_STACK="$PWD $DIR_STACK"
    # Change to the new directory if one is provided
    [ $# -gt 0 ] && cd "$1"
}

# Function to emulate popd behavior
popd() {
    # Extract the top directory from the stack
    top_dir=${DIR_STACK%% *}
    # Remove the top directory from the stack
    DIR_STACK=${DIR_STACK#* }
    # Change to the extracted directory if it's not empty
    if [ -n "$top_dir" ]; then
        cd "$top_dir"
    else
        echo "Directory stack is empty."
    fi
}

# check for `unzip`
command -v unzip >/dev/null 2>&1 || {
  echo "Error: package 'unzip' not found. Please install it." >&2
  exit 1
}

TOFU_VERSION="1.6.0-alpha2"

OS="$(uname | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m | sed -e 's/aarch64/arm64/' -e 's/x86_64/amd64/')"
TEMPDIR="$(mktemp -d)"
trap 'rm -rf "${TEMPDIR}"' EXIT # Cleanup on exit

pushd "${TEMPDIR}" >/dev/null 

echo "Downloading OpenTofu..."
wget "https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_${OS}_${ARCH}.zip" || {
  echo "Error downloading OpenTofu. Please check the provided version and your internet connection." >&2
  exit 1
}
unzip "tofu_${TOFU_VERSION}_${OS}_${ARCH}.zip" || {
  echo "Error unzipping OpenTofu. The downloaded file may be corrupted." >&2
  exit 1
}
sudo mv tofu /usr/local/bin/tofu || {
  echo "Error installing OpenTofu. Please ensure you have sufficient permissions." >&2
  exit 1
}
popd >/dev/null

echo "OpenTofu is now available at /usr/local/bin/tofu."