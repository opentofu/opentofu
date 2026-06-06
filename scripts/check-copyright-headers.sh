#!/usr/bin/env bash
# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Checks that all .go and .proto files carry the correct dual copyright header.
# Skips generated files (those containing "Code generated" in the first line).
# Can be run locally: bash scripts/check-copyright-headers.sh
# Exit code 1 if any file is missing or has a wrong header.

set -euo pipefail

EXPECTED_HEADER="// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0"

HEADER_LINES=$(echo "$EXPECTED_HEADER" | wc -l)

is_generated() {
  grep -qE '(^.{1,2} Code generated .* DO NOT EDIT\.\r?$)' "$1"
}

mismatched=()

while IFS= read -r file; do
  if is_generated "$file"; then
    continue
  fi

  file_header=$(head -n "$HEADER_LINES" "$file")

  if [ "$file_header" != "$EXPECTED_HEADER" ]; then
    mismatched+=("$file")
  fi
done < <(find . \
  -path './.git' -prune -o \
  -path './vendor' -prune -o \
  -path './website' -prune -o \
  \( -name '*.go' -o -name '*.proto' \) -print)

if [ "${#mismatched[@]}" -gt 0 ]; then
  echo >&2 "ERROR: the following files are missing or have an incorrect copyright header:"
  for f in "${mismatched[@]}"; do
    echo >&2 "  $f"
  done
  echo >&2 ""
  echo >&2 "Run scripts/add-copyright-headers.sh to add missing headers, or manually"
  echo >&2 "add the following to the top of each file:"
  echo >&2 ""
  echo >&2 "$EXPECTED_HEADER"
  exit 1
fi

echo "All files have correct copyright headers (scanned $(find . \
  -path './.git' -prune -o \
  -path './vendor' -prune -o \
  -path './website' -prune -o \
  \( -name '*.go' -o -name '*.proto' \) -print | wc -l | tr -d ' ') files)."
