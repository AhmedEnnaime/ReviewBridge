BINARY_NAME=reviewbridge
CMD_PATH=./cmd/reviewbridge
BIN_DIR=./bin
DIST_DIR=./dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags="-s -w -X main.version=$(VERSION)"

.PHONY: build build-docker test test-build test-cover test-integration test-e2e lint clean install release dev help

build:
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

build-docker:
	docker build -f Dockerfile.build \
	  --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 \
	  -o $(BIN_DIR)/ .

test:
	go test ./... -race -timeout 60s

test-build:
	go test ./tests/build/... -v -timeout 300s

test-cover:
	go test ./... -coverprofile=coverage.out -timeout 60s
	go tool cover -html=coverage.out -o coverage.html

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

test-e2e:
	sh tests/e2e/setup.sh

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) coverage.out coverage.html

install: build
	cp $(BIN_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

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

dev:
	docker compose -f docker-compose.dev.yml up

help:
	@echo "Targets: build build-docker test test-build test-cover test-integration test-e2e lint clean install release dev"
