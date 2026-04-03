SHELL := /bin/zsh

GO ?= go
NPM ?= npm
WIN7_GO ?= ./.tools/bin/go1.20.14
UI_DIR := ui
BIN_DIR := bin
BIN := $(BIN_DIR)/stock-sim
WIN_BIN := $(BIN_DIR)/stock-sim-windows-amd64.exe
WIN7_BIN := $(BIN_DIR)/stock-sim-win7.exe

.PHONY: dev-api dev-ui dev build-ui build build-win7 test test-go test-ui clean

dev-api:
	SIM_DB_PATH=../stock-scanner/data/scanner.sqlite $(GO) run .

dev-ui:
	cd $(UI_DIR) && $(NPM) run dev

dev:
	trap 'kill 0' EXIT; $(MAKE) dev-api & $(MAKE) dev-ui & wait

build-ui:
	cd $(UI_DIR) && $(NPM) run build

build: build-ui
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN) .
	GOOS=windows GOARCH=amd64 $(GO) build -o $(WIN_BIN) .

build-win7: build-ui
	mkdir -p $(BIN_DIR)
	$(WIN7_GO) test ./...
	GOOS=windows GOARCH=amd64 $(WIN7_GO) build -o $(WIN7_BIN) .

test: test-go test-ui

test-go:
	$(GO) test ./...

test-ui:
	cd $(UI_DIR) && $(NPM) run test

clean:
	rm -rf $(BIN_DIR) $(UI_DIR)/node_modules internal/httpapi/dist
