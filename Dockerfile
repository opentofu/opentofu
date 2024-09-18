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
            && echo -e "\033[1;33mfor your own builds. This image is only intended as a command line tool\033[0m" >&2 \
            && echo -e "\033[1;33mand not as general-purpose base image. It is not safe to use to build\033[0m" >&2 \
            && echo -e "\033[1;33mservices on top of because we don't regularly ship updates to all\033[0m" >&2 \
            && echo -e "\033[1;33mpackages in this image, which would be required for a secure base\033[0m" >&2 \
            && echo -e "\033[1;33mimage.\033[0m" >&2 \
            && echo -e "\033[1;33m\033[0m" >&2 \
            && echo -e "\033[1;33mStarting with OpenTofu 1.10, this image will refuse to build if used\033[0m" >&2 \
            && echo -e "\033[1;33mas a base image. Please follow the instructions at\033[0m" >&2 \
            && echo -e "\033[1;33m https://opentofu.org/docs/intro/install/docker/ to build your own\033[0m" >&2 \
            && echo -e "\033[1;33mimage. See https://github.com/opentofu/opentofu/issues/1931 for details\033[0m" >&2 \
            && echo -e "\033[1;33mon this decision.\033[0m" >&2

ONBUILD RUN # WARNING! PLEASE READ!
ONBUILD RUN # Please read carefully: you are using the OpenTofu image as a base image
ONBUILD RUN # for your own builds. This image is only intended as a command line tool
ONBUILD RUN # and not as general-purpose base image. It is not safe to use to build
ONBUILD RUN # services on top of because we don't regularly ship updates to all
ONBUILD RUN # packages in this image, which would be required for a secure base
ONBUILD RUN # image.
ONBUILD RUN # Starting with OpenTofu 1.10, this image will refuse to build if used
ONBUILD RUN # as a base image. Please follow the instructions at
ONBUILD RUN # https://opentofu.org/docs/intro/install/docker/ to build your own
ONBUILD RUN # image. See https://github.com/opentofu/opentofu/issues/1931 for details
ONBUILD RUN # on this decision.

# This sleep is here to hopefully catch people's attention due to increased build times.
ONBUILD RUN sleep 120

ENTRYPOINT ["/usr/local/bin/tofu"]
