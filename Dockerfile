# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

FROM alpine:3.20

LABEL maintainer="OpenTofu Core Team <core@opentofu.org>"

RUN apk add --no-cache git bash openssh

COPY tofu /usr/local/bin/tofu

ONBUILD RUN echo -e "\033[1;33mWARNING! PLEASE READ!\033[0m" >&2 \
            && echo -e "\033[1;33mPlease read carefully: you are using the OpenTofu image as a base image\033[0m" >&2 \
            && echo -e "\033[1;33mfor your own builds. This is no longer supported as of OpenTofu 1.10.\033[0m" >&2 \
            && echo -e "\033[1;33mPlease follow the instructions at\033[0m" >&2 \
            && echo -e "\033[1;33m https://opentofu.org/docs/intro/install/docker/ to build your own\033[0m" >&2 \
            && echo -e "\033[1;33mimage. See https://github.com/opentofu/opentofu/issues/1931 for details\033[0m" >&2 \
            && echo -e "\033[1;33mon this decision.\033[0m" >&2

ONBUILD RUN exit 1

ENTRYPOINT ["/usr/local/bin/tofu"]
