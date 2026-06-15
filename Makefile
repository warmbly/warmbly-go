# Makefile for warmbly-go — the official Go SDK for Warmbly.
#
# Common entry points:
#   make            # show this help
#   make check      # fmt + vet + lint + test (run before committing)
#
# Requires: Go 1.23+. The `lint` target additionally requires golangci-lint,
# and `fmt` will use goimports when it is available on PATH.

GO        ?= go
GOLANGCI  ?= golangci-lint

.DEFAULT_GOAL := help

.PHONY: help build test cover lint fmt vet tidy check clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Compile all packages
	$(GO) build ./...

test: ## Run tests with the race detector
	$(GO) test -race ./...

cover: ## Run tests with coverage and print a per-function report
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

lint: ## Run golangci-lint
	$(GOLANGCI) run

fmt: ## Format code (gofmt -s, plus goimports if available)
	gofmt -s -w .
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w -local github.com/warmbly/warmbly-go .; \
	else \
		echo "goimports not found; skipping (run: go install golang.org/x/tools/cmd/goimports@latest)"; \
	fi

vet: ## Run go vet
	$(GO) vet ./...

tidy: ## Tidy and verify the module graph
	$(GO) mod tidy

check: fmt vet lint test ## Run fmt, vet, lint and test

clean: ## Remove build and coverage artifacts
	$(GO) clean ./...
	rm -f coverage.out
