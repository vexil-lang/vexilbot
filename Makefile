.PHONY: build test lint

build:
	go build -o bin/vexilbot ./cmd/vexilbot

test:
	go test ./...

lint:
	go vet ./...
	golangci-lint run
