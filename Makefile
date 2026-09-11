.PHONY: build test lint run dump

build:
	go build -o bin/pyragit ./cmd/pyragit

test:
	go test ./...

lint:
	go vet ./...

run:
	go run ./cmd/pyragit

dump:
	go run ./cmd/pyragit --dump
