#!/bin/bash

set -e

if [ -f /usr/bin/zypper ]; then
  zypper install -y sudo
  if [ "$1" = "--convenience" ]; then
    bash -ex rpm-convenience-zypper.sh
  else
    bash -ex repo-zypper.sh
    bash -ex install-zypper.sh
  fi
else
  yum install -y sudo
  if [ "$1" = "--convenience" ]; then
    bash -ex rpm-convenience-yum.sh
  else
    bash -ex repo-yum.sh
    bash -ex install-yum.sh
  fi
fi

tofu --version