.PHONY: build test run fmt lint tidy e2e

build:
	go build -o pigo .

test:
	go test ./...

# Live tests against the configured model. Skips when OPENAI_API_KEY or
# OPENAI_BASE_URL is unset. Hermetic tests run as part of `make test`.
e2e:
	go test -tags e2e -count=1 -timeout 15m ./e2e/live/

run:
	go run .

fmt:
	gofmt -w .

lint:
	golangci-lint run ./...

# Fail when go.mod or go.sum would change. Does not rewrite them.
tidy:
	go mod tidy -diff
