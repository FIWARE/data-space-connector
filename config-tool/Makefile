MODULE      := github.com/FIWARE/data-space-connector/config-tool
BINARY      := dsc-config
BIN_DIR     := bin
TAG_PREFIX  := config-tool/
VERSION     ?= $(patsubst $(TAG_PREFIX)%,%,$(shell git describe --tags --match '$(TAG_PREFIX)v*' 2>/dev/null || echo dev))
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS     := -s -w -X $(MODULE)/internal/version.Version=$(VERSION) -X $(MODULE)/internal/version.Commit=$(COMMIT)
# A file list, not directories: gofmt recurses into ui/node_modules and ignores go.mod's ignore.
# wildcard drops tracked files deleted from the working tree, which gofmt would fail on.
GO_FILES     = $(wildcard $(shell git ls-files --cached --others --exclude-standard '*.go'))

.PHONY: build test lint

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) ./cmd/dsc-config

test:
	go test ./...

lint:
	go vet ./...
	$(if $(GO_FILES),,@echo "warning: git listed no Go files; gofmt check skipped" >&2)
	@unformatted="$$($(if $(GO_FILES),gofmt -l $(GO_FILES)))"; \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
