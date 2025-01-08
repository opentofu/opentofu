#!/bin/sh
# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

set -e

# Output the header as a single line:
echo '{"magic":"OpenTofu-External-Key-Provider","version":1}'

# Read the input metadata.
INPUT=$(printf '%s' "$(cat)")

if [ "${INPUT}" = "null" ]; then
  # We don't have metadata and shouldn't output a decryption key.
  cat << EOF
{
  "keys":{
    "encryption_key":"AQIDBAUGBwgJCgsMDQ4PEA=="
  },
  "meta":{
    "external_data":{}
  }
}
EOF
else
  # We have metadata and should output a decryption key. In our simplified case it's the
  # same as the encryption key.
  cat << EOF
{
  "keys":{
    "encryption_key":"AQIDBAUGBwgJCgsMDQ4PEA==",
    "decryption_key":"AQIDBAUGBwgJCgsMDQ4PEA=="
  },
  "meta":{
    "external_data":{}
  }
}
EOF
fi
