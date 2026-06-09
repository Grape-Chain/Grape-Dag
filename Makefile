GO      ?= go
DOCKER  ?= docker
MODULE  := github.com/Grape-Chain/Grape-Dag
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X $(MODULE)/version.Version=$(VERSION)

.PHONY: help build test lint vet fmt docker compose-up compose-down clean

help:  ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build all binaries into ./bin/
	$(GO) build -ldflags="$(LDFLAGS)" -o bin/ ./cmd/...

test:  ## Run unit tests with race detector
	$(GO) test -race -cover ./...

vet:   ## Run go vet
	$(GO) vet ./...

fmt:   ## Check formatting (fails if any file needs gofmt)
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "Files need gofmt:"; echo "$$out"; exit 1; fi

lint: vet fmt ## Run all static checks

docker: ## Build the peer Docker image
	$(DOCKER) build -t grape-peer:$(VERSION) --build-arg VERSION=$(VERSION) -f deploy/Dockerfile .

compose-up: ## Bring up the local stack (peer + smc + mongo)
	$(DOCKER) compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build

compose-down: ## Tear down the local stack
	$(DOCKER) compose -f deploy/docker-compose.yml down

clean: ## Remove build artifacts
	rm -rf bin/ dist/ build/ coverage.* *.out
