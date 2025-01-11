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
        sys.stderr.write("This is an OpenTofu key provider and is not meant to be run interactively. "
                         "Please configure this program in your OpenTofu encryption block to use it.\n")
        sys.exit(1)

    # Write the header:
    sys.stdout.write((json.dumps({"magic": "OpenTofu-External-Key-Provider", "version": 1}) + "\n"))

    # Read the input:
    inputData = sys.stdin.read()
    data = json.loads(inputData)

    # Construct the key:
    key = b''
    for i in range(1, 17):
        key += chr(i).encode('ascii')

    # Output the keys:
    if data is None:
        # No input metadata was passed, we shouldn't output a decryption key. If needed, we can produce
        # an output metadata here, which will be stored alongside the encrypted data.
        outputMeta = {"external_data":{}}
        sys.stdout.write(json.dumps({
            "keys": {
                "encryption_key": base64.b64encode(key).decode('ascii')
            },
            "meta": outputMeta
        }))
    else:
        # We had some input metadata, output a decryption key. In a real-life scenario we would
        # use the metadata for something like pbdkf2.
        inputMeta = data["external_data"]
        # Do something with the input metadata if needed and produce the output metadata:
        outputMeta = {"external_data":{}}
        sys.stdout.write(json.dumps({
            "keys": {
                "encryption_key": base64.b64encode(key).decode('ascii'),
                "decryption_key": base64.b64encode(key).decode('ascii')
            },
            "meta": outputMeta
        }))
