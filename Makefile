# # /bin/sh does not support source command needed in make test
#SHELL := /bin/bash

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
GOLANGCI_LINT_VERSION ?= v1.50.1
ENVTEST_K8S_VERSION = 1.26.1

.PHONY: golanci-lint
golanci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(LOCALBIN) $(GOLANGCI_LINT_VERSION)

go-lint: golanci-lint ## Run golangci-lint against code.
	$(GOLANGCI_LINT) run

test: envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -i --bin-dir $(LOCALBIN) -p path)" go run github.com/onsi/ginkgo/v2/ginkgo -r pkg/ -coverprofile cover.out -progress -v -timeout=12h0m0s -focus=${FOCUS} --junit-report="junit.xml"  -- ${ARGS}

ENVTEST ?= $(LOCALBIN)/setup-envtest

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: build
build:
	go build -o bin/akoctl main.go

.PHONY: goreleaser-install
goreleaser-install: $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/goreleaser/goreleaser@latest

.PHONY: gorelease
gorelease: goreleaser-install
	goreleaser release --snapshot --clean