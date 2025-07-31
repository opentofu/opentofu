#!/usr/bin/python
# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

import base64
import json
import sys

if __name__ == "__main__":
    # Make sure that this program isn't running interactively:
    if sys.stdout.isatty():
        sys.stderr.write("This is an OpenTofu encryption method and is not meant to be run interactively. "
                         "Please configure this program in your OpenTofu encryption block to use it.\n")
        sys.exit(1)

    # Write the header:
    sys.stdout.write((json.dumps({"magic": "OpenTofu-External-Encryption-Method", "version": 1}) + "\n"))
    sys.stdout.flush()

    # Read the input:
    inputData = sys.stdin.read()
    data = json.loads(inputData)

    key = base64.b64decode(data["key"])
    payload = base64.b64decode(data["payload"])

    # Encrypt the data:
    outputPayload = bytearray()
    for i in range(0, len(payload)):
        b = payload[i]
        outputPayload.append(key[i%len(key)] ^ b)

    # Write the output:
    sys.stdout.write(json.dumps({
        "payload": base64.b64encode(outputPayload).decode('ascii'),
    }))
