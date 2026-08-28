GO      ?= go
DOCKER  ?= docker
MODULE  := github.com/Grape-Chain/Grape-Dag
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X $(MODULE)/version.Version=$(VERSION)

WALLET_DIR := web/wallet

.PHONY: help build test lint vet fmt docker compose-up compose-down clean wallet wallet-clean \
	txgen bench-node-max bench-node-rate bench-txgen

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

wallet: ## Build the web wallet signer (WebAssembly) into web/wallet/
	@echo "building $(WALLET_DIR)/wallet.wasm"
	GOOS=js GOARCH=wasm $(GO) build -ldflags="-s -w" -o $(WALLET_DIR)/wallet.wasm ./cmd/walletwasm
	@cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" $(WALLET_DIR)/wasm_exec.js 2>/dev/null \
	  || cp "$$($(GO) env GOROOT)/misc/wasm/wasm_exec.js" $(WALLET_DIR)/wasm_exec.js
	@gzip -9 -c $(WALLET_DIR)/wallet.wasm > $(WALLET_DIR)/wallet.wasm.gz
	@ls -lh $(WALLET_DIR)/wallet.wasm $(WALLET_DIR)/wallet.wasm.gz | awk '{print "  " $$9 "  " $$5}'

wallet-clean: ## Remove built wallet assets
	rm -f $(WALLET_DIR)/wallet.wasm $(WALLET_DIR)/wallet.wasm.gz

docker: ## Build the peer Docker image
	$(DOCKER) build -t grape-peer:$(VERSION) --build-arg VERSION=$(VERSION) -f deploy/Dockerfile .

compose-up: ## Bring up the local stack (peer + smc + mongo)
	$(DOCKER) compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build

compose-down: ## Tear down the local stack
	$(DOCKER) compose -f deploy/docker-compose.yml down

bench: ## Run the micro-benchmarks for tip selection and the commit path
	go test ./dag/ -run XXXNOMATCH -bench . -benchtime 200x -benchmem

bench-commit: ## Benchmark only the synchronous commit path (fsync, settled replay, slice)
	go test ./dag/ -run XXXNOMATCH -bench 'PinCommit|StoreAppend|SettledApply|BalanceSnapshot' -benchtime 200x -benchmem

bench-select: ## Benchmark only tip selection and graph growth
	go test ./dag/ -run XXXNOMATCH -bench 'TipSelection|GraphGrowth|ConfirmTracker' -benchtime 500x -benchmem

# ---------------------------------------------------------------- load testing
# TXGEN_PORT is the gRPC port of the node under test. TXGEN_ARGS passes anything
# else through, e.g. TXGEN_ARGS="-bench_workers 64 -bench_duration 60s".
TXGEN_PORT ?= 50333
TXGEN_ARGS ?=

txgen: ## Build only the transaction generator into ./bin/
	$(GO) build -ldflags="$(LDFLAGS)" -o bin/ ./cmd/txgen

bench-node-max: txgen ## Saturate the node at TXGEN_PORT to find its maximum sustained throughput
	./bin/txgen -mode bench -grpc_port $(TXGEN_PORT) -bench_max $(TXGEN_ARGS)

bench-node-rate: txgen ## Offer a fixed rate to the node at TXGEN_PORT: make bench-node-rate RATE=5000
	@test -n "$(RATE)" || (echo "set RATE, e.g. make bench-node-rate RATE=5000"; exit 1)
	./bin/txgen -mode bench -grpc_port $(TXGEN_PORT) -bench_rate $(RATE) $(TXGEN_ARGS)

bench-txgen: ## Run the txgen unit tests, pacing and metrics included
	$(GO) test -race -count=1 ./tools/txgen/

clean: ## Remove build artifacts
	rm -rf bin/ dist/ build/ coverage.* *.out
