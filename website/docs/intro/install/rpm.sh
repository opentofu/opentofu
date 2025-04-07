#!/bin/bash
# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


set -e

if [ -f /usr/bin/zypper ]; then
  zypper install -y sudo
  if [ "$1" = "--convenience" ]; then
    bash -ex examples/rpm-convenience.sh
  else
    bash -ex examples/repo-zypper.sh
    bash -ex examples/install-zypper.sh
  fi
else
  yum install -y sudo
  if [ "$1" = "--convenience" ]; then
    bash -ex examples/rpm-convenience.sh
  else
    bash -ex examples/repo-yum.sh
    bash -ex examples/install-yum.sh
  fi
fi

tofu -version