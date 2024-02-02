#!/usr/bin/env bash
# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

set -uo pipefail

function usage {
  cat <<-'EOF'
Usage: ./equivalence-test.sh <command> [<args>] [<options>]

Description:
  This script will handle various commands related to the execution of the
  tofu equivalence tests.

Commands:
  download_equivalence_test_binary <version> <target> <os> <arch>
    download_equivalence_test_binary downloads the equivalence testing binary
    for a given version and places it at the target path.

    ./equivalence-test.sh download_equivalence_test_binary 0.4.0 ./bin/equivalence-testing linux amd64
EOF
}

function download_equivalence_test_binary {
  VERSION="${1:-}"
  TARGET="${2:-}"
  OS="${3:-}"
  ARCH="${4:-}"

  if [[ -z "$VERSION" || -z "$TARGET" || -z "$OS" || -z "$ARCH" ]]; then
    echo "missing at least one of [<version>, <target>, <os>, <arch>] arguments"
    usage
    exit 1
  fi

  curl \
    -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/opentofu/equivalence-testing/releases" > releases.json

  ASSET="equivalence-testing_v${VERSION}_${OS}_${ARCH}.zip"
  ASSET_ID=$(jq -r --arg VERSION "v$VERSION" --arg ASSET "$ASSET" '.[] | select(.name == $VERSION) | .assets[] | select(.name == $ASSET) | .id' releases.json)

  mkdir -p zip
  curl -L \
    -H "Accept: application/octet-stream" \
    "https://api.github.com/repos/opentofu/equivalence-testing/releases/assets/$ASSET_ID" > "zip/$ASSET"

  mkdir -p bin
  unzip -p "zip/$ASSET" equivalence-testing > "$TARGET"
  chmod u+x "$TARGET"
  rm -r zip
  rm releases.json
}

function main {
  case "$1" in
    download_equivalence_test_binary)
      if [ "${#@}" != 5 ]; then
        echo "invalid number of arguments"
        usage
        exit 1
      fi

      download_equivalence_test_binary "$2" "$3" "$4" "$5"

      ;;
    *)
      echo "unrecognized command $*"
      usage
      exit 1

      ;;
  esac
}

main "$@"
exit $?
