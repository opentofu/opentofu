#!/bin/sh

set -e

apk add curl

if [ "$1" = "--convenience" ]; then
  sh -x alpine-convenience.sh
else
  sh -x alpine-manual.sh
fi

tofu --version