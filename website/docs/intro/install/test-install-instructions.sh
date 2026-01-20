#!/bin/bash
# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


# This script tests the installation instructions on all relevant Linux operating systems listed in docker-compose.yaml.

set -eo pipefail

TEMPFILE=$(mktemp)

set +e

docker compose down >/dev/null 2>&1
docker compose up $1 >$TEMPFILE 2>&1
EXIT_CODE=$?

if [ "${EXIT_CODE}" -ne 0 ]; then
  echo -e "\033[0;31mFailed to execute docker compose up\033[0m"
  cat $TEMPFILE >&2
  rm $TEMPFILE
  exit "${EXIT_CODE}"
fi

SERVICES=$(docker compose ps -a --format '{{.Service}}')
FINAL_EXIT_CODE=0
FAILED=0
for SERVICE in $SERVICES; do
  EXIT_CODE=$(docker compose ps -a --format '{{.Service}}\t{{.ExitCode}}' | grep -E "^${SERVICE}\s" | cut -f 2)
  if [ "${EXIT_CODE}" -eq 0 ]; then
    echo -e "::group::\033[0;32m✅  ${SERVICE}\033[0m"
  else
    echo -e "::group::\033[0;31m❌  ${SERVICE}\033[0m"
    docker compose logs ${SERVICE}
    FAILED=$((${FAILED}+1))
  fi
  cat $TEMPFILE | grep -a -E "^${SERVICE}-1\s+\| " | sed -E "s/^${SERVICE}-1\s+\| //"
  echo "::endgroup::"
done

if [ "${FAILED}" -ne 0 ]; then
  echo -e "::group::\033[0;31m❌  Summary (${FAILED} failed)\033[0m"
else
  echo -e "::group::\033[0;32m✅  Summary (all tests passed)\033[0m"
fi
echo -en "\033[1m"
printf '%-32s%s\n' 'Test case' 'Result (exit code)'
echo -en "\033[0m"
for SERVICE in $SERVICES; do
  EXIT_CODE=$(docker compose ps -a --format '{{.Service}}\t{{.ExitCode}}' | grep -E "^${SERVICE}\s" | cut -f 2)
  if [ "${EXIT_CODE}" -eq 0 ]; then
    RESULT=$(echo -ne "\033[0;32mpass (${EXIT_CODE})\033[0m")
  else
    RESULT=$(echo -ne "\033[0;31mfail (${EXIT_CODE})\033[0m")
  fi
  printf '%-32s%s\n' "${SERVICE}" "${RESULT}"
done
echo "::endgroup::"

docker compose down >/dev/null 2>&1
rm $TEMPFILE
if [ "${FAILED}" -ne 0 ]; then
  exit 1
fi
exit 0