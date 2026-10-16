include ../../scripts/go.mk

CHECK_TARGETS := tidy fmt-check lint vuln unit-test migrations-check build

.PHONY: generate build run migrate migrate-up migrate-status migrate-hash migrations-check integration-test test-all docker-build

CONFIG ?= config.yaml
TEST_CONFIG ?= config.test.yaml
MIGRATION_CONFIG ?= migrations.yaml
ATLAS ?= atlas
YQ ?= $(if $(wildcard $(HOME)/go/bin/yq),$(HOME)/go/bin/yq,yq)

generate:
	go generate -v ./ent ./internal/core/usecases/... ./internal/rest/...

build:
	go build -o bin/payment -v ./cmd/payment

run:
	go run ./cmd/payment --config "$(CONFIG)"

migrate migrate-up:
	@set -eu; \
	dsn="$$( $(YQ) -er '.database.url' "$(CONFIG)" )"; \
	dir="$$( $(YQ) -er '.directory' "$(MIGRATION_CONFIG)" )"; \
	lock="$$( $(YQ) -er '.lock_timeout' "$(MIGRATION_CONFIG)" )"; \
	duration="$$( $(YQ) -er '.timeout' "$(MIGRATION_CONFIG)" )"; \
	timeout "$$duration" $(ATLAS) migrate apply --dir "file://$$dir" --url "$$dsn" --lock-timeout "$$lock"

migrate-status:
	@set -eu; \
	dsn="$$( $(YQ) -er '.database.url' "$(CONFIG)" )"; \
	dir="$$( $(YQ) -er '.directory' "$(MIGRATION_CONFIG)" )"; \
	duration="$$( $(YQ) -er '.timeout' "$(MIGRATION_CONFIG)" )"; \
	timeout "$$duration" $(ATLAS) migrate status --dir "file://$$dir" --url "$$dsn"

migrate-hash:
	@set -eu; \
	dir="$$( $(YQ) -er '.directory' "$(MIGRATION_CONFIG)" )"; \
	$(ATLAS) migrate hash --dir "file://$$dir"

migrations-check:
	@set -eu; \
	dir="$$( $(YQ) -er '.directory' "$(MIGRATION_CONFIG)" )"; \
	$(ATLAS) migrate validate --dir "file://$$dir"

integration-test:
	mkdir -p bin
	YQ="$(YQ)" MIGRATION_CONFIG="$(MIGRATION_CONFIG)" ATLAS="$(ATLAS)" \
		sh ../../scripts/integration-test.sh "$(TEST_CONFIG)" $(GOTESTSUM) --junitfile bin/integration.xml $(TEST_REPORT_ARGS) -- -tags=integration -count=1 -race -timeout 3m ./tests/integration/...

test-all: unit-test integration-test

docker-build:
	docker build -f Dockerfile -t rsoi-payment-service ../..
