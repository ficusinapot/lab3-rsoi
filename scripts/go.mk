.DEFAULT_GOAL := build
override export GOWORK := off

GOLANGCI_LINT ?= $(if $(wildcard $(HOME)/go/bin/golangci-lint),$(HOME)/go/bin/golangci-lint,golangci-lint)
GOTESTSUM ?= $(if $(wildcard $(HOME)/go/bin/gotestsum),$(HOME)/go/bin/gotestsum,gotestsum)
GOVULNCHECK ?= $(if $(wildcard $(HOME)/go/bin/govulncheck),$(HOME)/go/bin/govulncheck,govulncheck)
TAGS ?=
CHECK_TARGETS ?= tidy fmt-check lint vuln unit-test build

# Formatting reads build tags from GOFLAGS; keep other caller-supplied flags.
ifneq ($(strip $(TAGS)),)
override export GOFLAGS := $(GOFLAGS) -tags=$(TAGS)
endif

.PHONY: unit-test test lint lint-fix style-check fmt fmt-check tidy vuln check

unit-test:
	mkdir -p bin
	$(GOTESTSUM) --junitfile bin/unit.xml $(TEST_REPORT_ARGS) -- -race -timeout 30s ./...

test: unit-test

lint:
	$(GOLANGCI_LINT) run $(if $(TAGS),--build-tags=$(TAGS)) -c .golangci.yml ./...

lint-fix:
	$(GOLANGCI_LINT) run --fix $(if $(TAGS),--build-tags=$(TAGS)) -c .golangci.yml ./...

style-check:
	$(GOLANGCI_LINT) run --enable-only lll $(if $(TAGS),--build-tags=$(TAGS)) -c .golangci.yml ./...

fmt:
	$(GOLANGCI_LINT) fmt -c .golangci.yml ./...

fmt-check:
	$(GOLANGCI_LINT) fmt --diff -c .golangci.yml ./...

tidy:
	go mod tidy --diff

vuln:
	$(GOVULNCHECK) -show verbose ./...

# Separate invocations preserve order even when the caller uses make -j.
check:
	+@set -eu; for target in $(CHECK_TARGETS); do $(MAKE) "$$target"; done
