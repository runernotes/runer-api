.PHONY: lint test build verify

lint:
	golangci-lint run

test:
	go test -v -race -coverprofile=coverage.out ./...

build:
	go build -v ./cmd/server/...

verify: lint test build
	@echo "All checks passed."
