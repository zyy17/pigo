.PHONY: build test run fmt lint

build:
	go build -o pigo .

test:
	go test ./...

run:
	go run .

fmt:
	gofmt -w .

lint:
	golangci-lint run ./...
