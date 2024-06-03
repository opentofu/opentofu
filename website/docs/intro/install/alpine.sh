#!/bin/sh
# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


set -e

apk add curl

if [ "$1" = "--convenience" ]; then
  sh -x examples/alpine-convenience.sh
else
  sh -x examples/alpine-manual.sh
fi

tofu --version