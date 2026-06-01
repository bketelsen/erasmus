BINARY ?= erasmus
MAIN ?= ./cmd/erasmus
DIST ?= dist
BREW_PREFIX ?= /home/linuxbrew/.linuxbrew

GO ?= $(shell if command -v go >/dev/null 2>&1; then command -v go; elif [ -x "$(BREW_PREFIX)/bin/go" ]; then printf '%s' "$(BREW_PREFIX)/bin/go"; else printf '%s' go; fi)
GOLANGCI_LINT ?= $(shell if command -v golangci-lint >/dev/null 2>&1; then command -v golangci-lint; elif [ -x "$(BREW_PREFIX)/bin/golangci-lint" ]; then printf '%s' "$(BREW_PREFIX)/bin/golangci-lint"; else printf '%s' golangci-lint; fi)
GORELEASER ?= $(shell if command -v goreleaser >/dev/null 2>&1; then command -v goreleaser; elif [ -x "$(BREW_PREFIX)/bin/goreleaser" ]; then printf '%s' "$(BREW_PREFIX)/bin/goreleaser"; else printf '%s' goreleaser; fi)

GOCACHE ?= /tmp/erasmus-gocache
GOLANGCI_LINT_CACHE ?= /tmp/erasmus-golangci-cache

.DEFAULT_GOAL := help

.PHONY: help paths doctor all ci test test-examples examples vet lint smoke build fmt tidy clean release-snapshot check-go check-golangci-lint check-goreleaser

help:
	@printf '%s\n' "Erasmus development targets"
	@printf '%s\n' ""
	@printf '%s\n' "Usage:"
	@printf '%s\n' "  make <target> [GO=/path/to/go] [GOLANGCI_LINT=/path/to/golangci-lint]"
	@printf '%s\n' ""
	@printf '%s\n' "Common targets:"
	@printf '%s\n' "  make test              run Go tests"
	@printf '%s\n' "  make test-examples     test and build all examples"
	@printf '%s\n' "  make lint              run golangci-lint"
	@printf '%s\n' "  make smoke             run scripts/test smoke coverage"
	@printf '%s\n' "  make build             build ./$(BINARY)"
	@printf '%s\n' "  make ci                run test, vet, lint, smoke, examples, and build"
	@printf '%s\n' "  make fmt               gofmt all Go packages"
	@printf '%s\n' "  make tidy              run go mod tidy"
	@printf '%s\n' "  make doctor            show resolved tool paths and versions"
	@printf '%s\n' "  make paths             show overridable paths"
	@printf '%s\n' ""
	@printf '%s\n' "Codex/local notes:"
	@printf '%s\n' "  - Linuxbrew Go is auto-detected at $(BREW_PREFIX)/bin/go when plain go is not on PATH."
	@printf '%s\n' "  - Caches default to /tmp so sandboxed/read-only home cache directories do not break builds."

paths:
	@printf '%s\n' "BINARY=$(BINARY)"
	@printf '%s\n' "MAIN=$(MAIN)"
	@printf '%s\n' "DIST=$(DIST)"
	@printf '%s\n' "BREW_PREFIX=$(BREW_PREFIX)"
	@printf '%s\n' "GO=$(GO)"
	@printf '%s\n' "GOLANGCI_LINT=$(GOLANGCI_LINT)"
	@printf '%s\n' "GORELEASER=$(GORELEASER)"
	@printf '%s\n' "GOCACHE=$(GOCACHE)"
	@printf '%s\n' "GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE)"

doctor:
	@$(MAKE) --no-print-directory paths
	@printf '%s' "go version: "
	@if [ -x "$(GO)" ] || command -v "$(GO)" >/dev/null 2>&1; then GOCACHE="$(GOCACHE)" "$(GO)" version; else printf '%s\n' "missing"; fi
	@printf '%s' "golangci-lint version: "
	@if [ -x "$(GOLANGCI_LINT)" ] || command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1; then GOCACHE="$(GOCACHE)" GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" "$(GOLANGCI_LINT)" version; else printf '%s\n' "missing"; fi
	@printf '%s' "goreleaser version: "
	@if [ -x "$(GORELEASER)" ] || command -v "$(GORELEASER)" >/dev/null 2>&1; then "$(GORELEASER)" --version; else printf '%s\n' "missing"; fi

all: test build

ci: test vet lint smoke test-examples build

test: check-go
	@GOCACHE="$(GOCACHE)" "$(GO)" test ./...

test-examples: check-go
	@go_path="$$(if [ -x "$(GO)" ]; then printf '%s' "$(GO)"; else command -v "$(GO)"; fi)"; \
		export PATH="$$(dirname "$$go_path"):$(BREW_PREFIX)/bin:$$PATH"; \
		GO="$$go_path" GOCACHE="$(GOCACHE)" scripts/test-examples

examples: test-examples

vet: check-go
	@GOCACHE="$(GOCACHE)" "$(GO)" vet ./...

lint: check-go check-golangci-lint
	@go_path="$$(if [ -x "$(GO)" ]; then printf '%s' "$(GO)"; else command -v "$(GO)"; fi)"; \
		export PATH="$$(dirname "$$go_path"):$(BREW_PREFIX)/bin:$$PATH"; \
		GOCACHE="$(GOCACHE)" GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" "$(GOLANGCI_LINT)" run ./...

smoke: check-go
	@go_path="$$(if [ -x "$(GO)" ]; then printf '%s' "$(GO)"; else command -v "$(GO)"; fi)"; \
		export PATH="$$(dirname "$$go_path"):$(BREW_PREFIX)/bin:$$PATH"; \
		GOCACHE="$(GOCACHE)" scripts/test

build: check-go
	@GOCACHE="$(GOCACHE)" "$(GO)" build -buildvcs=false -o "$(BINARY)" "$(MAIN)"

fmt: check-go
	@GOCACHE="$(GOCACHE)" "$(GO)" fmt ./...

tidy: check-go
	@GOCACHE="$(GOCACHE)" "$(GO)" mod tidy

clean:
	rm -rf "$(DIST)" "$(BINARY)" "$(BINARY).exe"

release-snapshot: check-goreleaser
	@"$(GORELEASER)" release --snapshot --clean

check-go:
	@if ! ([ -x "$(GO)" ] || command -v "$(GO)" >/dev/null 2>&1); then \
		printf '%s\n' "error: Go toolchain not found."; \
		printf '%s\n' ""; \
		printf '%s\n' "Install Go:"; \
		printf '%s\n' "  brew install go"; \
		printf '%s\n' "  sudo apt-get install golang-go"; \
		printf '%s\n' "  https://go.dev/doc/install"; \
		printf '%s\n' ""; \
		printf '%s\n' "Or rerun with an explicit path:"; \
		printf '%s\n' "  make GO=/path/to/go <target>"; \
		printf '%s\n' "  make GO=$(BREW_PREFIX)/bin/go <target>"; \
		exit 127; \
	fi

check-golangci-lint:
	@if ! ([ -x "$(GOLANGCI_LINT)" ] || command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1); then \
		printf '%s\n' "error: golangci-lint not found."; \
		printf '%s\n' ""; \
		printf '%s\n' "Install golangci-lint:"; \
		printf '%s\n' "  brew install golangci-lint"; \
		printf '%s\n' "  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; \
		printf '%s\n' "  https://golangci-lint.run/welcome/install/"; \
		printf '%s\n' ""; \
		printf '%s\n' "Or rerun with an explicit path:"; \
		printf '%s\n' "  make GOLANGCI_LINT=/path/to/golangci-lint lint"; \
		printf '%s\n' "  make GOLANGCI_LINT=$(BREW_PREFIX)/bin/golangci-lint lint"; \
		exit 127; \
	fi

check-goreleaser:
	@if ! ([ -x "$(GORELEASER)" ] || command -v "$(GORELEASER)" >/dev/null 2>&1); then \
		printf '%s\n' "error: goreleaser not found."; \
		printf '%s\n' ""; \
		printf '%s\n' "Install GoReleaser:"; \
		printf '%s\n' "  brew install goreleaser"; \
		printf '%s\n' "  https://goreleaser.com/install/"; \
		printf '%s\n' ""; \
		printf '%s\n' "Or rerun with an explicit path:"; \
		printf '%s\n' "  make GORELEASER=/path/to/goreleaser release-snapshot"; \
		exit 127; \
	fi
