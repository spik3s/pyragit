.PHONY: build test lint fmt run dump

build:
	go build -o bin/pyragit ./cmd/pyragit

test:
	go test ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

fmt:
	gofmt -w .

run:
	go run ./cmd/pyragit

dump:
	go run ./cmd/pyragit --dump
