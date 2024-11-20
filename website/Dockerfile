# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

FROM alpine AS parent

RUN apk add git
RUN git clone --recurse-submodules https://github.com/opentofu/opentofu.org /work

FROM node:22
COPY --from=parent /work /work
WORKDIR /work
RUN npm i

VOLUME /work/opentofu-repo/main
EXPOSE 3000

CMD ["npm", "run", "start", "--", "--host","0.0.0.0"]
