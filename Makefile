SHELL := /bin/zsh

GO ?= go
NPM ?= npm
WAILS ?= $(HOME)/go/bin/wails
WIN7_GO_VERSION ?= 1.20.14
WIN7_GO_HOST_OS := $(shell cd /tmp && $(GO) env GOHOSTOS)
WIN7_GO_HOST_ARCH := $(shell cd /tmp && $(GO) env GOHOSTARCH)
WIN7_GO_BIN := $(HOME)/go/pkg/mod/golang.org/toolchain@v0.0.1-go$(WIN7_GO_VERSION).$(WIN7_GO_HOST_OS)-$(WIN7_GO_HOST_ARCH)/bin/go
UI_DIR := ui
BIN_DIR := bin
# Outputs of `make build` (Wails multi-platform names the Windows exe stock-sim-amd64.exe)
BIN_MACOS_WAILS := $(BIN_DIR)/stock-sim-macos.app
BIN_WIN_WAILS := $(BIN_DIR)/stock-sim-windows.exe
BIN_WIN7 := $(BIN_DIR)/stock-sim-win7.exe
SERVER_BIN := $(BIN_DIR)/stock-sim-server
WIN_BIN := $(BIN_DIR)/stock-sim-windows-amd64.exe

.PHONY: dev dev-browser dev-api dev-ui dev-wails ui-install win7-go build-ui build build-server build-desktop build-win7 test test-go test-ui clean

# Wails desktop (fixed Vite 5173 + API 3002 when SIM_ADDR is set in env / wails.json)
dev-wails:
	SIM_ADDR=127.0.0.1:3002 $(WAILS) dev

dev: dev-wails

# Browser + embedded server (no Wails)
dev-browser:
	trap 'kill 0' EXIT; $(MAKE) dev-api & $(MAKE) dev-ui & wait

dev-api:
	SIM_DB_PATH=../stock-scanner/data/scanner.sqlite $(GO) run ./cmd/server

ui-install:
	@if [ ! -d "$(UI_DIR)/node_modules" ]; then \
		cd $(UI_DIR) && $(NPM) ci; \
	fi

dev-ui: ui-install
	cd $(UI_DIR) && $(NPM) run dev

build-ui: ui-install
	cd $(UI_DIR) && $(NPM) run build

# Quick single-platform Wails build -> build/bin/ (for local iteration)
build-desktop: build-ui
	$(WAILS) build

win7-go:
	@(cd /tmp && GOTOOLCHAIN=go$(WIN7_GO_VERSION)+auto $(GO) version >/dev/null)
	@if [ ! -f "go.win7.sum" ]; then cp go.sum go.win7.sum; fi

# All release-style artifacts into bin/: macOS Wails .app, Windows Wails .exe, Win7/html cmd/server .exe
build: build-ui win7-go
	rm -rf $(BIN_MACOS_WAILS) $(BIN_WIN_WAILS) $(BIN_WIN7)
	mkdir -p $(BIN_DIR)
	$(WAILS) build -platform darwin/arm64,windows/amd64
	cp -R build/bin/stock-sim.app $(BIN_MACOS_WAILS)
	cp build/bin/stock-sim-amd64.exe $(BIN_WIN_WAILS)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(WIN7_GO_BIN) build -modfile=go.win7.mod -mod=mod -o $(BIN_WIN7) ./cmd/server

# Legacy: static server binaries only (no Wails)
build-server: build-ui
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(SERVER_BIN) ./cmd/server
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -o $(WIN_BIN) ./cmd/server

build-win7: build-ui win7-go
	mkdir -p $(BIN_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(WIN7_GO_BIN) build -modfile=go.win7.mod -mod=mod -o $(BIN_WIN7) ./cmd/server

test: test-go test-ui

test-go:
	$(GO) test ./...

test-ui: ui-install
	cd $(UI_DIR) && $(NPM) run test

clean:
	rm -rf $(BIN_DIR) $(UI_DIR)/node_modules internal/httpapi/dist build/bin build/darwin build/windows
