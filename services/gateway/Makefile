include ../../scripts/go.mk

.PHONY: build run generate integration-test e2e-test test-all docker-build

CONFIG ?= config.yaml

build:
	go build -o bin/gateway -v ./cmd/gateway

run:
	go run ./cmd/gateway --config "$(CONFIG)"

generate:
	go generate ./internal/rest/bookings/api ./internal/clients/contracts ./internal/core/usecases/bookings ./internal/rest/bookings

integration-test:
	mkdir -p bin
	$(GOTESTSUM) --junitfile bin/integration.xml $(TEST_REPORT_ARGS) -- -tags=integration -race -timeout 30s ./tests/integration/...

e2e-test:
	mkdir -p bin
	GATEWAY_PROJECT_ROOT="$(abspath ../..)" $(GOTESTSUM) --junitfile bin/e2e.xml $(TEST_REPORT_ARGS) -- -tags=e2e -count=1 -timeout 10m ./tests/e2e/...

test-all: unit-test integration-test e2e-test

docker-build:
	docker build -f Dockerfile -t rsoi-gateway-service ../..
