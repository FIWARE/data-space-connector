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

UI_DIR       := ui
# npm ci writes this file, so make reinstalls only when the lockfile changes.
UI_DEPS      := $(UI_DIR)/node_modules/.package-lock.json
# Release targets: pure Go, so one job cross-compiles them all (ADR 0011).
CROSS_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
# The native window needs CGO and the OS webview SDK; desktop,production are Wails' own tags.
WINDOW_TAGS_BASE := wails,desktop,production
ifeq ($(shell go env GOOS),linux)
WINDOW_TAGS  := $(WINDOW_TAGS_BASE),webkit2_41
else
WINDOW_TAGS  := $(WINDOW_TAGS_BASE)
endif

.PHONY: ui build build-window cross test lint

$(UI_DEPS): $(UI_DIR)/package-lock.json
	cd $(UI_DIR) && npm ci

ui: $(UI_DEPS)
	cd $(UI_DIR) && npm run build

build: ui
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) ./cmd/dsc-config

build-window: ui
	CGO_ENABLED=1 go build -trimpath -tags '$(WINDOW_TAGS)' -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY)-window ./cmd/dsc-config

cross: ui
	@set -e; for target in $(CROSS_TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; ext=; [ "$$os" = windows ] && ext=.exe; \
		echo "build $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' \
			-o $(BIN_DIR)/$(BINARY)-$$os-$$arch$$ext ./cmd/dsc-config; \
	done

test: $(UI_DEPS)
	go test ./...
	cd $(UI_DIR) && npm test

lint:
	go vet ./...
	$(if $(GO_FILES),,@echo "warning: git listed no Go files; gofmt check skipped" >&2)
	@unformatted="$$($(if $(GO_FILES),gofmt -l $(GO_FILES)))"; \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
