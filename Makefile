.PHONY: build test check run

build:
	go build -o bin/mud-server ./cmd/mud-server

test:
	go test ./...

check:
	go vet ./...
	go test ./...

run:
	go run ./cmd/mud-server
