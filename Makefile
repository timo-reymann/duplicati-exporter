.PHONY: help clean test coverage test-coverage-report save-coverage-report lint fmt \
	create-dist build-linux build-windows build-darwin build-freebsd build-openbsd \
	build-docker create-checksums build run

SHELL := /bin/bash

MODULE := github.com/timo-reymann/duplicati-exporter
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
NOW := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
COMMIT_REF := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BIN_PREFIX := dist/duplicati-exporter_
IMAGE := timoreymann/duplicati-exporter
PLATFORMS := linux/amd64,linux/arm/v7,linux/arm64

BUILD_ARGS := -ldflags "-s -w \
	-X $(MODULE)/internal/buildinfo.Version=$(VERSION) \
	-X $(MODULE)/internal/buildinfo.GitSha=$(COMMIT_REF) \
	-X $(MODULE)/internal/buildinfo.BuildTime=$(NOW)"

help: ## Display this help page
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[33m%-30s\033[0m %s\n", $$1, $$2}'

clean: ## Remove build artifacts
	@rm -rf dist/ coverage.txt coverage.html junit.xml

fmt: ## Format all Go sources
	@gofmt -w .

lint: ## Run go vet
	@go vet ./...

test: ## Run all tests
	@CGO_ENABLED=0 go test -race -count=1 ./...

coverage: ## Run tests and measure coverage
	@CGO_ENABLED=0 go test -covermode=count -coverprofile=/tmp/count.out -v ./...

test-coverage-report: coverage ## Run tests and open the coverage report in a browser
	@go tool cover -html=/tmp/count.out

save-coverage-report: coverage ## Write the coverage report to coverage.html
	@go tool cover -html=/tmp/count.out -o coverage.html

create-dist: ## Create the dist folder if it does not exist
	@mkdir -p dist/

build-linux: create-dist ## Cross-compile for Linux
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN_PREFIX)linux-amd64 $(BUILD_ARGS) ./cmd/duplicati-exporter
	@CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -o $(BIN_PREFIX)linux-386 $(BUILD_ARGS) ./cmd/duplicati-exporter
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm go build -o $(BIN_PREFIX)linux-arm $(BUILD_ARGS) ./cmd/duplicati-exporter
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(BIN_PREFIX)linux-arm64 $(BUILD_ARGS) ./cmd/duplicati-exporter

build-windows: create-dist ## Cross-compile for Windows
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BIN_PREFIX)windows-amd64.exe $(BUILD_ARGS) ./cmd/duplicati-exporter
	@CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -o $(BIN_PREFIX)windows-386.exe $(BUILD_ARGS) ./cmd/duplicati-exporter
	@CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o $(BIN_PREFIX)windows-arm64.exe $(BUILD_ARGS) ./cmd/duplicati-exporter

build-darwin: create-dist ## Cross-compile for macOS
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(BIN_PREFIX)darwin-amd64 $(BUILD_ARGS) ./cmd/duplicati-exporter
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(BIN_PREFIX)darwin-arm64 $(BUILD_ARGS) ./cmd/duplicati-exporter

build-freebsd: create-dist ## Cross-compile for FreeBSD
	@CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 go build -o $(BIN_PREFIX)freebsd-amd64 $(BUILD_ARGS) ./cmd/duplicati-exporter
	@CGO_ENABLED=0 GOOS=freebsd GOARCH=386 go build -o $(BIN_PREFIX)freebsd-386 $(BUILD_ARGS) ./cmd/duplicati-exporter
	@CGO_ENABLED=0 GOOS=freebsd GOARCH=arm64 go build -o $(BIN_PREFIX)freebsd-arm64 $(BUILD_ARGS) ./cmd/duplicati-exporter

build-openbsd: create-dist ## Cross-compile for OpenBSD
	@CGO_ENABLED=0 GOOS=openbsd GOARCH=amd64 go build -o $(BIN_PREFIX)openbsd-amd64 $(BUILD_ARGS) ./cmd/duplicati-exporter
	@CGO_ENABLED=0 GOOS=openbsd GOARCH=386 go build -o $(BIN_PREFIX)openbsd-386 $(BUILD_ARGS) ./cmd/duplicati-exporter

build: build-linux build-darwin build-windows build-freebsd build-openbsd create-checksums ## Cross-compile for every supported platform

create-checksums: ## Write a sha256 next to every binary in dist/
	@find ./dist -type f ! -name '*.sha256' -exec sh -c '\
		f="$$1"; \
		if command -v sha256sum >/dev/null 2>&1; then \
			sha256sum "$$f" | cut -d " " -f 1 > "$$f.sha256"; \
		else \
			shasum -a 256 "$$f" | cut -d " " -f 1 > "$$f.sha256"; \
		fi' _ {} \;

build-docker: ## Build and push the multi-arch container image
	@docker buildx build --tag $(IMAGE):latest \
		--platform $(PLATFORMS) \
		--build-arg BUILD_TIME="$(NOW)" \
		--build-arg BUILD_VERSION="$(VERSION)" \
		--build-arg BUILD_COMMIT_REF="$(COMMIT_REF)" \
		--push .
	@docker buildx build --tag $(IMAGE):$(VERSION) \
		--platform $(PLATFORMS) \
		--build-arg BUILD_TIME="$(NOW)" \
		--build-arg BUILD_VERSION="$(VERSION)" \
		--build-arg BUILD_COMMIT_REF="$(COMMIT_REF)" \
		--push .

run: ## Run the exporter locally
	@go run ./cmd/duplicati-exporter $(ARGS)
