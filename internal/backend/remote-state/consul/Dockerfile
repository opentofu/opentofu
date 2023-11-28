FROM consul:1.15

# install dependencies to install Go
RUN apk add -q curl tar

# download Go
ARG GO_VERSION
RUN if [ "$(uname -m)"=="aarch64" ]; then curl -SLo /tmp/go.tar.gz https://go.dev/dl/go${GO_VERSION}.linux-arm64.tar.gz;\
      elif [ "$(uname -m)"=="arm64" ]; then curl -SLo /tmp/go.tar.gz https://go.dev/dl/go${GO_VERSION}.linux-arm64.tar.gz;\
      elif [ "$(uname -m)"=="arm" ]; then curl -SLo /tmp/go.tar.gz https://go.dev/dl/go${GO_VERSION}.linux-armv6l.tar.gz;\
      else curl -SLo /tmp/go.tar.gz https://go.dev/dl/go${GO_VERSION}.linux-386.tar.gz; \
    fi

# install Go
RUN cd tmp && \
    tar -xzf go.tar.gz && \
    ln -s /tmp/go/bin/go /usr/local/bin/go && \
    rm go.tar.gz

# cleanup installation dependencies
RUN apk del -q curl tar

VOLUME "/app"
WORKDIR "/app"

ENV TF_CONSUL_TEST=1
ENTRYPOINT [ "go" ]
