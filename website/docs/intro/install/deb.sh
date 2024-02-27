#!/bin/bash

set -e

apt update
apt install -y sudo curl

if [ "$1" = "--convenience" ]; then
  bash -ex deb-convenience.sh
else
  bash -ex deb-step1.sh
  bash -ex deb-step2.sh
  bash -ex deb-step3.sh
  bash -ex deb-step4.sh
fi

tofu --version