#!/bin/bash
# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


set -e

if [ -f /usr/bin/zypper ]; then
  zypper install -y sudo
  if [ "$1" = "--convenience" ]; then
    bash -ex rpm-convenience.sh
  else
    bash -ex repo-zypper.sh
    bash -ex install-zypper.sh
  fi
else
  yum install -y sudo
  if [ "$1" = "--convenience" ]; then
    bash -ex rpm-convenience.sh
  else
    bash -ex repo-yum.sh
    bash -ex install-yum.sh
  fi
fi

tofu --version