.PHONY: lint test e2e build run docker-build docker-run verify

lint:
	golangci-lint run

test:
	go test -v -race -coverprofile=coverage.out $(shell go list ./... | grep -v /e2e)

e2e:
	go test -v -timeout 120s ./e2e/...

build:
	go build -v ./cmd/server/...

docker-build:
	docker build -t runer-api .

clean:
	rm server

run:
	go run ./cmd/server/...

docker-run:
	docker run --rm -p 8080:8080 --env-file .env runer-api

verify: lint test e2e build clean
	@echo "All checks passed."
