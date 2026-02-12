# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

FROM alpine:3.20

LABEL maintainer="Ghoten <https://github.com/vmvarela/opentofu>"

RUN apk add --no-cache git bash openssh

ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/ghoten /usr/local/bin/ghoten

ENTRYPOINT ["/usr/local/bin/ghoten"]
