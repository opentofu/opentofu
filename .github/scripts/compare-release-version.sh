#!/usr/bin/env bash
# SPDX-License-Identifier: MPL-2.0

# Compare the semver tag against the current release in the VERSION file

set -uo pipefail

# Bail if VERSION cannot be found
if [[ ! -f version/VERSION ]]; then
    echo "The VERSION file could not be found. Please create a VERSION file in the version/ directory. The contents of version should match the tag without the v prefix."
    exit 1
fi

if [ -z "$TARGET_VERSION" ]; then
    echo "The TARGET_VERSION environment variable is not set."
    exit 1
fi

# Create a clean semver tag without the v prefix
CLEAN_TAG=$(echo "$TARGET_VERSION" | sed 's/^v//')

VERSION=$(cat version/VERSION)
if [[ "$VERSION" != "$CLEAN_TAG" ]]; then
    echo "The VERSION file does not match the tag. Please update the version/VERSION file to match the tag without the v prefix."
    echo "The VERSION file contains: $VERSION but the tag is: $CLEAN_TAG."
    exit 1
fi

echo "The VERSION file matches the tag. Proceeding with the release of $VERSION."
