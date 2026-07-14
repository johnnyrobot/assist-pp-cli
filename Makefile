.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/assist-pp-cli$(BIN_EXT) ./cmd/assist-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/assist-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/assist-pp-mcp$(BIN_EXT) ./cmd/assist-pp-mcp

install-mcp:
	go install ./cmd/assist-pp-mcp

build-all: build build-mcp
