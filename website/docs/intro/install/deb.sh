#!/bin/bash
# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


set -e

apt update
apt install -y sudo curl

if [ "$1" = "--convenience" ]; then
  bash -ex examples/deb-convenience.sh
else
  bash -ex examples/deb-step1.sh
  bash -ex examples/deb-step2.sh
  bash -ex examples/deb-step3.sh
  bash -ex examples/deb-step4.sh
fi

tofu -version