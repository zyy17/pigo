.PHONY: build test run fmt

build:
	go build -o pigo .

test:
	go test ./...

run:
	go run .

fmt:
	gofmt -w .
