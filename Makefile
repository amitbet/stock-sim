SHELL := /bin/zsh

GO ?= go
NPM ?= npm
UI_DIR := ui
BIN_DIR := bin
BIN := $(BIN_DIR)/stock-sim

.PHONY: dev-api dev-ui dev build-ui build test test-go test-ui clean

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

test: test-go test-ui

test-go:
	$(GO) test ./...

test-ui:
	cd $(UI_DIR) && $(NPM) run test

clean:
	rm -rf $(BIN_DIR) $(UI_DIR)/node_modules $(UI_DIR)/dist
