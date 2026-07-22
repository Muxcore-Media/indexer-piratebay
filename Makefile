.PHONY: build test lint clean fmt tidy docker ci help

GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
LDFLAGS ?= -s -w -X main.version=$(VERSION)
BINARY ?= indexer-piratebay

build:
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/module

test:
	CGO_ENABLED=0 $(GO) test -count=1 -timeout 60s ./...

lint:
	golangci-lint run --timeout 120s ./...

clean:
	rm -f $(BINARY)
	rm -f cmd/module/module
	rm -rf dist/

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

docker:
	docker build -t ghcr.io/muxcore-media/$(BINARY):$(VERSION) .

ci: test build

help:
	@echo "Targets: build test lint clean fmt tidy docker ci"
