#!/bin/bash
# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


tofu version 2>/dev/null >/dev/null
if [ $? -ne 0 ]; then
  set -e
  TOFU_VERSION="1.6.0-alpha2"
  OS="$(uname | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m | sed -e 's/aarch64/arm64/' -e 's/x86_64/amd64/')"
  TEMPDIR="$(mktemp -d)"
  pushd "${TEMPDIR}" >/dev/null
  wget "https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_${OS}_${ARCH}.zip"
  unzip "tofu_${TOFU_VERSION}_${OS}_${ARCH}.zip"
  sudo mv tofu /usr/local/bin/tofu
  popd >/dev/null
  rm -rf "${TEMPDIR}"
  set +e
fi

ERROR=0
for testcase in $(ls -d */); do
  testcase=$(echo -n "${testcase}" | sed -e 's$/$$')
  (
    cd $testcase
    tofu init
    RESULT=$?
    if [ "$RESULT" -ne 0 ]; then
      exit "$RESULT"
    fi
    tofu test
    exit $?
  ) 2>/tmp/${testcase}.log >/tmp/${testcase}.log
  RESULT=$?
  echo -n "::group::"
  if [ "${RESULT}" -ne 0 ]; then
    echo -e "\033[0;31m${testcase} (FAIL)\033[0m"
    ERROR="${RESULT}"
  else
    echo -e "\033[0;32m${testcase} (PASS)\033[0m"
  fi
  cat /tmp/${testcase}.log
  echo "::endgroup::"
done

exit "${ERROR}"
