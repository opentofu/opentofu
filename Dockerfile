# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

FROM alpine:3.18

LABEL maintainer="OpenTofu Core Team <core@opentofu.org>"

COPY tofu /usr/local/bin/tofu

ENTRYPOINT ["/usr/local/bin/tofu"]
