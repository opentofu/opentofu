.POSIX:

export PATH := $(abspath bin/):${PATH}

# Dependency versions
LICENSEI_VERSION = 0.9.0

# run Go tests and generate a coverage report
#
# NOTE:
#   The coverage counters are to be updated atomically,
#   which is useful for tests that run in parallel.
.PHONY: test-with-coverage
test-with-coverage:
	go test -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html

# run the unit tests across all packages in the tofu project
.PHONY: test
test:
	go test -v ./...

EXT := $(shell go env GOEXE)

# build tofu binary in the current directory with the version set to the git tag
# or commit hash if there is no tag.
.PHONY: build
build:
	go build -ldflags "-X main.version=$(shell git describe --tags --always --dirty)" -o tofu$(EXT) ./cmd/tofu

# generate runs `go generate` to build the dynamically generated
# source files, except the protobuf stubs which are built instead with
# "make protobuf".
.PHONY: generate
generate:
	go generate ./...

# We separate the protobuf generation because most development tasks on
# OpenTofu do not involve changing protobuf files and protoc is not a
# go-gettable dependency and so getting it installed can be inconvenient.
#
# If you are working on changes to protobuf interfaces, run this Makefile
# target to be sure to regenerate all of the protobuf stubs using the expected
# versions of protoc and the protoc Go plugins.
.PHONY: protobuf
protobuf:
	go run ./tools/protobuf-compile .

# Golangci-lint
.PHONY: golangci-lint
golangci-lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.0 run --timeout 60m ./...

# Run license check
.PHONY: license-check
license-check:
	go mod vendor
	licensei cache --debug
	licensei check --debug
	licensei header --debug
	rm -rf vendor/
	git diff --exit-code

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

# Integration tests
#

.PHONY: list-integration-tests
list-integration-tests: ## Lists tests.
	@ grep -h -E '^(test|integration)-.+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[1m%-30s\033[0m %s\n", $$1, $$2}'

# integration test with s3 as backend
.PHONY: test-s3

define infoTestS3
 Test requires:
 * AWS Credentials to be configured
   - https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html
   - https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-envvars.html
 * IAM Permissions in us-west-2
   - S3 CRUD operations on buckets which will follow the pattern tofu-test-*
   - DynamoDB CRUD operations on a Table named dynamoTable

endef

test-s3: ## Runs tests with s3 bucket as the backend.
	@ $(info $(infoTestS3))
	@ TF_S3_TEST=1 go test ./internal/backend/remote-state/s3/...

# integration test with gcp as backend
.PHONY: test-gcp

define infoTestGCP
 This test requires a working set of default credentials on the host.
 You can configure those by running `gcloud auth application-default login`.
 Additionally, you'll need to set the following environment variables:
 - GOOGLE_REGION to a valid GCP region, e.g. us-west1
 - GOOGLE_PROJECT to a valid GCP project ID

 Note: The GCP tests leave behind a keyring, because those can't easily be deleted. It will be reused across test runs.

endef

test-gcp: ## Runs tests with gcp as the backend.
	@ $(info $(infoTestGCP))
	@ TF_ACC=1 go test ./internal/backend/remote-state/gcs/...
	@ echo "Note: this test has left behind a keyring, because those can't easily be deleted. It will be reused across test runs."

# integration test with postgres as backend
.PHONY: test-pg test-pg-clean

PG_PORT := 5432

define infoTestPg
 Test requires:
 * Docker: https://docs.docker.com/engine/install/
 * Port: $(PG_PORT)

endef

test-pg: ## Runs tests with local Postgres instance as the backend.
	@ $(info $(infoTestPg))
	@ echo "Starting database"
	@ make test-pg-clean
	@ docker run --rm -d --name tofu-pg \
        -p $(PG_PORT):5432 \
        -e POSTGRES_PASSWORD=tofu \
        -e POSTGRES_USER=tofu \
        postgres:16-alpine3.17 1> /dev/null
	@ docker exec tofu-pg /bin/bash -c 'until psql -U tofu -c "\q" 2> /dev/null; do echo "Database is getting ready, waiting"; sleep 1; done'
	@ DATABASE_URL="postgres://tofu:tofu@localhost:$(PG_PORT)/tofu?sslmode=disable" \
 		TF_PG_TEST=1 go test ./internal/backend/remote-state/pg/...

test-pg-clean: ## Cleans environment after `test-pg`.
	@ docker rm -f tofu-pg 2> /dev/null

# integration test with Azure as backend
.PHONY: test-azure

test-azure: ## Directs the developer to follow a runbook describing how to run Azure integration tests.
	@ echo "To run Azure integration tests, please follow the runbook in internal/backend/remote-state/azure/README.md".
	@ exit 1 # don't want the user to miss this

# integration test with Consul as backend
.PHONY: test-consul test-consul-clean

define infoTestConsul
 Test requires:
 * Docker: https://docs.docker.com/engine/install/

endef

GO_VER := `cat $(PWD)/.go-version`

test-consul: ## Runs tests using in docker container using Consul as the backend.
	@ $(info $(infoTestConsul))
	@ echo "Build docker image with Consul and Go v$(GO_VER)"
	@ cd ./internal/backend/remote-state/consul &&\
  		docker build --build-arg="GO_VERSION=${GO_VER}" -t tofu-consul --progress=plain . &> /dev/null
	@ echo "Run tests"
	@ docker run --rm --name tofu-consul -v $(PWD):/app -e TF_CONSUL_TEST=1 -t tofu-consul \
 		test ./internal/backend/remote-state/consul/...

test-consul-clean: ## Cleans environment after `test-consul`.
	@ docker rmi -f tofu-consul:latest

# integration test with kubernetes as backend
.PHONY: test-kubernetes test-kubernetes-clean

define infoTestK8s
 Test requires:
 * Git client
 * Docker: https://docs.docker.com/engine/install/
 Note! Please make sure that the docker configurations satisfy requirements: https://kind.sigs.k8s.io/docs/user/quick-start#settings-for-docker-desktop

endef

KIND_VERSION := v0.20.0

test-kubernetes: test-kubernetes-clean ## Runs tests with a local kubernetes cluster as the backend.
	@ $(info $(infoTestK8s))
	@ echo "Installing KinD $(KIND_VERSION): https://kind.sigs.k8s.io/docs/user/quick-start/#installing-with-make"
	@ git clone -c advice.detachedHead=false -q https://github.com/kubernetes-sigs/kind -b $(KIND_VERSION) /tmp/kind-repo 1> /dev/null && \
 		 cd /tmp/kind-repo &&\
 		 make build 1> /dev/null &&\
 		 mv ./bin/kind /tmp/tofuk8s &&\
 		 cd .. && rm -rf kind-repo
	@ echo "Provisioning a cluster"
	@ /tmp/tofuk8s -q create cluster --name tofu-kubernetes
	@ /tmp/tofuk8s -q export kubeconfig --name tofu-kubernetes --kubeconfig /tmp/tofu-k8s-config
	@ echo "Running tests"
	@ KUBE_CONFIG_PATHS=/tmp/tofu-k8s-config TF_K8S_TEST=1 go test ./internal/backend/remote-state/kubernetes/...
	@ echo "Deleting provisioned cluster"
	@ make test-kubernetes-clean

test-kubernetes-clean: ## Cleans environment after `test-kubernetes`.
	@ test -s /tmp/tofu-k8s-config && rm /tmp/tofu-k8s-config || echo "" > /dev/null
	@ test -s /tmp/tofuk8s && (/tmp/tofuk8s -q delete cluster --name tofu-kubernetes && rm /tmp/tofuk8s) || echo "" > /dev/null

.PHONY:
test-linux-install-instructions:
	@cd "$(CURDIR)/website/docs/intro/install" && ./test-install-instructions.sh

.PHONY:
integration-tests: test-s3 test-pg test-consul test-kubernetes integration-tests-clean ## Runs all integration tests test.

.PHONY:
integration-tests-clean: test-pg-clean test-consul-clean test-kubernetes-clean ## Cleans environment after all integration tests.

.PHONY: help
help: ## Prints this help message.
	@echo ""
	@echo "Opentofu Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "The available targets for execution are listed below."
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*$$"; OFS = ""} \
    /^# .*$$/ { doc=$$0; sub(/^# /, "", doc); next } \
    /^[a-zA-Z0-9_-]+:.*## .*$$/ { target=$$1; sub(/:$$/, "", target); desc=$$0; sub(/^[^#]*## /, "", desc); if (!seen[target]++) { printf "\033[1m%-30s\033[0m %s\n", target, desc } } \
    /^[a-zA-Z0-9_-]+:.*$$/ { target=$$1; sub(/:$$/, "", target); if (!seen[target]++) { if (doc != "") { printf "\033[1m%-30s\033[0m %s\n", target, doc; doc="" } else { printf "\033[1m%-30s\033[0m\n", target } } }' $(MAKEFILE_LIST)
