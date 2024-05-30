#!/bin/sh

set -e

apk add curl

if [ "$1" = "--convenience" ]; then
  sh -x examples/alpine-convenience.sh
else
  sh -x examples/alpine-manual.sh
fi

tofu --version