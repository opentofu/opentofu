export PATH := $(abspath bin/):${PATH}

# Dependency versions
LICENSEI_VERSION = 0.9.0

# generate runs `go generate` to build the dynamically generated
# source files, except the protobuf stubs which are built instead with
# "make protobuf".
.PHONY: generate
generate:
	go generate ./...

# We separate the protobuf generation because most development tasks on
# OpenTF do not involve changing protobuf files and protoc is not a
# go-gettable dependency and so getting it installed can be inconvenient.
#
# If you are working on changes to protobuf interfaces, run this Makefile
# target to be sure to regenerate all of the protobuf stubs using the expected
# versions of protoc and the protoc Go plugins.
.PHONY: protobuf
protobuf:
	go run ./tools/protobuf-compile .

.PHONY: fmtcheck
fmtcheck:
	"$(CURDIR)/scripts/gofmtcheck.sh"

.PHONY: importscheck
importscheck:
	"$(CURDIR)/scripts/goimportscheck.sh"

.PHONY: staticcheck
staticcheck:
	"$(CURDIR)/scripts/staticcheck.sh"

.PHONY: exhaustive
exhaustive:
	"$(CURDIR)/scripts/exhaustive.sh"

# Run this if working on the website locally to run in watch mode.
.PHONY: website
website:
	$(MAKE) -C website website

# Use this if you have run `website/build-local` to use the locally built image.
.PHONY: website/local
website/local:
	$(MAKE) -C website website/local

# Run this to generate a new local Docker image.
.PHONY: website/build-local
website/build-local:
	$(MAKE) -C website website/build-local

# Run license check
.PHONY: license-check
license-check:
	go mod vendor
	licensei check
	licensei header

# Install dependencies
deps: bin/licensei
deps:

bin/licensei: bin/licensei-${LICENSEI_VERSION}
	@ln -sf licensei-${LICENSEI_VERSION} bin/licensei
bin/licensei-${LICENSEI_VERSION}:
	@mkdir -p bin
	curl -sfL https://git.io/licensei | bash -s v${LICENSEI_VERSION}
	@mv bin/licensei $@

# disallow any parallelism (-j) for Make. This is necessary since some
# commands during the build process create temporary files that collide
# under parallel conditions.
.NOTPARALLEL:
