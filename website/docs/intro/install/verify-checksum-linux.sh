# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

ZIPFILE=tofu_*.zip
CHECKSUM=$(sha256sum "${ZIPFILE}" | cut -f 1 -d ' ')
EXPECTED_CHECKSUM=$(grep "${ZIPFILE}" tofu_*_SHA256SUMS | cut -f 1 -d ' ')
if [ "${CHECKSUM}" = "${EXPECTED_CHECKSUM}" ]; then
    echo "OK"
else
    echo "MISMATCH"
fi