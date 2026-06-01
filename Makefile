BINARY_NAME=reviewbridge
CMD_PATH=./cmd/reviewbridge
BIN_DIR=./bin
DIST_DIR=./dist

.PHONY: build build-docker test test-integration test-e2e lint clean install release help

## build: compile binary for current platform
build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

## build-docker: compile linux/amd64 binary via Docker (reproducible)
build-docker:
	docker build -f Dockerfile.build \
	  --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 \
	  -o $(BIN_DIR)/ .

## test: run all unit tests
test:
	go test ./... -race -timeout 60s

## test-cover: run unit tests with coverage report
test-cover:
	go test ./... -coverprofile=coverage.out -timeout 60s
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## test-integration: run integration tests using Docker WireMock (no real credentials needed)
test-integration:
	docker compose -f docker-compose.test.yml up -d
	@until curl -sf http://localhost:8080/__admin/health > /dev/null 2>&1; do sleep 1; done
	@until curl -sf http://localhost:8081/__admin/health > /dev/null 2>&1; do sleep 1; done
	REVIEWBRIDGE_GITHUB_BASE_URL=http://localhost:8080 \
	REVIEWBRIDGE_GITLAB_BASE_URL=http://localhost:8081 \
	go test ./tests/integration/... -v -timeout 120s; \
	EXIT_CODE=$$?; \
	docker compose -f docker-compose.test.yml down; \
	exit $$EXIT_CODE

## test-e2e: run end-to-end tests using Docker WireMock
test-e2e:
	sh tests/e2e/setup.sh

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) coverage.out coverage.html

## install: install binary to /usr/local/bin
install: build
	cp $(BIN_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

## release: build binaries for all platforms via Docker
release:
	@mkdir -p $(DIST_DIR)
	@for platform in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do \
	  OS=$$(echo $$platform | cut -d/ -f1); \
	  ARCH=$$(echo $$platform | cut -d/ -f2); \
	  echo "Building $$OS/$$ARCH..."; \
	  docker build -f Dockerfile.build \
	    --build-arg TARGETOS=$$OS \
	    --build-arg TARGETARCH=$$ARCH \
	    -o $(DIST_DIR)/$(BINARY_NAME)-$$OS-$$ARCH . ; \
	done
	@echo "Release binaries in $(DIST_DIR)/"

## dev: run daemon in Docker dev container
dev:
	docker compose -f docker-compose.dev.yml up

## help: show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
