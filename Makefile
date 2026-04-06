.PHONY: lint test e2e build dev docker-build docker-run verify

lint:
	golangci-lint run

test:
	go test -v -race -coverprofile=coverage.out $(shell go list ./... | grep -v /e2e)

e2e:
	go test -v -count=1 -timeout 120s ./e2e/...

build:
	go build -v ./cmd/server/...

docker-build:
	docker build -t runer-api .

clean:
	rm server

dev:
	go run ./cmd/server/...

docker-run:
	docker run --rm -p 8080:8080 --env-file .env runer-api

verify: lint test e2e build clean
	@echo "All checks passed."

# ---------------------------------------------------------------------------
# Release
# ---------------------------------------------------------------------------

release: guard-TAG
	git tag $(TAG)
	git push origin $(TAG)

guard-%:
	@[ -n "$($(*))" ] || (echo "Error: $* is required. Usage: make release TAG=v1.2.3"; exit 1)
