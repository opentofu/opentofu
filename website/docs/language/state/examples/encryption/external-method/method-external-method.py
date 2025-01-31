#!/usr/bin/python
import argparse
import base64
import json
import sys

if __name__ == "__main__":
    # Make sure that this program isn't running interactively:
    if sys.stdout.isatty():
        sys.stderr.write("This is an OpenTofu encryption method and is not meant to be run interactively. "
                         "Please configure this program in your OpenTofu encryption block to use it.\n")
        sys.exit(1)
    parser = argparse.ArgumentParser(prog='My External Encryption Method')
    parser.add_argument('--encrypt', action='store_true')
    parser.add_argument('--decrypt', action='store_true')
    args = parser.parse_args()

    # Write the header:
    sys.stdout.write((json.dumps({"magic": "OpenTofu-External-Encryption-Method", "version": 1}) + "\n"))
    sys.stdout.flush()

    # Read the input:
    inputData = sys.stdin.read()
    data = json.loads(inputData)

    key = base64.b64decode(data["key"])
    payload = base64.b64decode(data["payload"])

    # Produce the output payload here.
    if args.encrypt:
        outputPayload = b''
    elif args.decrypt:
        outputPayload = b''
    else:
        raise "Expected --encrypt or --decrypt."

    # Write the output:
    sys.stdout.write(json.dumps({
        "payload": base64.b64encode(outputPayload).decode('ascii'),
    }))
