#!/bin/bash

# This script tests the installation instructions on all relevant Linux operating systems listed in docker-compose.yaml.

set -e

docker compose down || true
docker compose up $1

echo -e "Test case\tExit code"
docker compose ps -a --format '{{ .Service}}\t{{.ExitCode}}' | tee /tmp/$$
FAILS=$(cat /tmp/$$ | cut -f 2 | grep -v '^0$' | wc -l)
rm /tmp/$$
if [ "${FAILS}" -ne 0 ]; then
  echo "${FAILS} tests failed." >&2
  exit 1
fi
docker compose down
echo "All tests passed."
