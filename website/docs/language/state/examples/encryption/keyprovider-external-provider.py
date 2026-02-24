#!/usr/bin/python

import base64
import json
import sys

if __name__ == "__main__":
    # Write the header:
    sys.stdout.write((json.dumps(
        {"magic": "OpenTofu-External-Key-Provider", "version": 1}) + "\n"
    ))
    sys.stdout.flush()

    # Read the input:
    inputData = sys.stdin.read()
    data = json.loads(inputData)

    # Construct the key:
    key = b'AQIDBAUGBwgJCgsMDQ4PEA=='

    # Output the keys:
    if data is None:
        # No input metadata was passed, we shouldn't output a decryption key.
        # If needed, we can produce an output metadata here, which will be
        # stored alongside the encrypted data.
        outputMeta = {"external_data":{}}
        sys.stdout.write(json.dumps({
            "keys": {
                "encryption_key": base64.b64encode(key).decode('ascii')
            },
            "meta": outputMeta
        }))
    else:
        # We had some input metadata, output a decryption key. In a real-life
        # scenario we would use the metadata for something like pbdkf2.
        inputMeta = data["external_data"]
        # Do something with the input metadata if needed and produce the output
        # metadata:
        outputMeta = {"external_data":{}}
        sys.stdout.write(json.dumps({
            "keys": {
                "encryption_key": base64.b64encode(key).decode('ascii'),
                "decryption_key": base64.b64encode(key).decode('ascii')
            },
            "meta": outputMeta
        }))
