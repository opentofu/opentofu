# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

echo '@testing https://dl-cdn.alpinelinux.org/alpine/edge/testing' >> /etc/apk/repositories
apk add opentofu@testing