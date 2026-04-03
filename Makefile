.PHONY: build run test server lint release-dry

## build: compile the curlx binary
build:
	go build -o bin/curlx ./cmd/curlx

## run: run curlx from source
run:
	go run ./cmd/curlx

## server: start the test server on :9090
server:
	go run ./testserver

## server2: start the task manager test server on :9091
server2:
	go run ./testserver2

## test: run all tests
test:
	go test ./...

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## tidy: tidy go modules
tidy:
	go mod tidy

## release-dry: dry-run GoReleaser to validate config
release-dry:
	goreleaser release --snapshot --clean

## help: print this help
help:
	@grep -E '^##' Makefile | sed 's/## //'
