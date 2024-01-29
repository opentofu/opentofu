#!/bin/bash

set -e

if command_exists "zypper"; then
  zypper install -y sudo
  if [ "$1" = "--convenience" ]; then
    bash -ex rpm-convenience.sh
  else
    bash -ex repo-zypper.sh
    bash -ex install-zypper.sh
  fi
elif command_exists "dnf"; then
  dnf install -y sudo
  if [ "$1" = "--convenience" ]; then
    bash -ex rpm-convenience.sh
  else
    bash -ex repo-yum.sh
    bash -ex install-dnf.sh
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
