.PHONY: lint test e2e build dev docker-build docker-run verify \
        version release-dev release-minor-dev release-prod release-patch release-minor

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
#
# Tagging convention:
#   v1.2.3        = production release
#   v1.2.3-dev.1  = dev/staging release
#
# Targets and what they produce depending on current state:
#
#   Target               From prod v1.2.3        From dev v1.2.3-dev.1
#   ───────────────────  ──────────────────────  ──────────────────────
#   release-dev          → v1.2.4-dev.1          → v1.2.3-dev.2
#   release-minor-dev    → v1.3.0-dev.1          ✗ error
#   release-prod         ✗ error                 → v1.2.3  (promote)
#   release-patch        → v1.2.4  (skip dev)    ✗ error
#   release-minor        → v1.3.0  (skip dev)    ✗ error
#
# Typical flow: release-dev (iterate) → release-prod (promote when ready)
# Escape hatch: release-patch / release-minor to go straight to prod
#
# Run 'make version' to see the current tag and preview all next versions.
# ---------------------------------------------------------------------------

# Print the current version and preview what each release target would produce.
version:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	base=$$(echo "$$latest" | sed -E 's/^v//; s/-dev\.[0-9]+$$//'); \
	major=$$(echo "$$base" | cut -d. -f1); \
	minor=$$(echo "$$base" | cut -d. -f2); \
	patch=$$(echo "$$base" | cut -d. -f3); \
	is_dev=$$(echo "$$latest" | grep -qE '\-dev\.' && echo 1 || echo 0); \
	dev_n=$$(echo "$$latest" | grep -oE '[0-9]+$$' 2>/dev/null || echo 0); \
	echo ""; \
	echo "  Current tag: $$latest"; \
	echo ""; \
	echo "  Target               Next tag"; \
	echo "  ───────────────────  ──────────────────────"; \
	if [ "$$is_dev" = "1" ]; then \
	  echo "  release-dev          v$$major.$$minor.$$patch-dev.$$((dev_n + 1))"; \
	  echo "  release-minor-dev    (error — already on dev)"; \
	  echo "  release-prod         v$$major.$$minor.$$patch  ← promote"; \
	  echo "  release-patch        (error — already on dev)"; \
	  echo "  release-minor        (error — already on dev)"; \
	else \
	  echo "  release-dev          v$$major.$$minor.$$((patch + 1))-dev.1"; \
	  echo "  release-minor-dev    v$$major.$$((minor + 1)).0-dev.1"; \
	  echo "  release-prod         (error — nothing to promote)"; \
	  echo "  release-patch        v$$major.$$minor.$$((patch + 1))  ← skip dev"; \
	  echo "  release-minor        v$$major.$$((minor + 1)).0  ← skip dev"; \
	fi; \
	echo ""

# Next dev build:
#   on prod v1.2.3       → v1.2.4-dev.1
#   on dev  v1.2.3-dev.1 → v1.2.3-dev.2
release-dev: verify
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	base=$$(echo "$$latest" | sed -E 's/^v//; s/-dev\.[0-9]+$$//'); \
	major=$$(echo "$$base" | cut -d. -f1); \
	minor=$$(echo "$$base" | cut -d. -f2); \
	patch=$$(echo "$$base" | cut -d. -f3); \
	is_dev=$$(echo "$$latest" | grep -qE '\-dev\.' && echo 1 || echo 0); \
	dev_n=$$(echo "$$latest" | grep -oE '[0-9]+$$' 2>/dev/null || echo 0); \
	if [ "$$is_dev" = "1" ]; then \
	  tag="v$$major.$$minor.$$patch-dev.$$((dev_n + 1))"; \
	else \
	  tag="v$$major.$$minor.$$((patch + 1))-dev.1"; \
	fi; \
	echo "Tagging: $$tag"; \
	git tag "$$tag" && git push origin "$$tag"

# Next minor as dev — must be on a prod tag:
#   v1.2.3 → v1.3.0-dev.1
release-minor-dev: verify
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	if echo "$$latest" | grep -qE '\-dev\.'; then \
	  echo "Error: already on a dev tag ($$latest). Promote to prod first with 'release-prod', or iterate with 'release-dev'."; \
	  exit 1; \
	fi; \
	base=$$(echo "$$latest" | sed 's/^v//'); \
	major=$$(echo "$$base" | cut -d. -f1); \
	minor=$$(echo "$$base" | cut -d. -f2); \
	tag="v$$major.$$((minor + 1)).0-dev.1"; \
	echo "Tagging: $$tag"; \
	git tag "$$tag" && git push origin "$$tag"

# Promote current dev tag to prod — must be on a dev tag:
#   v1.2.3-dev.N → v1.2.3
release-prod: verify
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	if ! echo "$$latest" | grep -qE '\-dev\.'; then \
	  echo "Error: not on a dev tag ($$latest). Nothing to promote."; \
	  exit 1; \
	fi; \
	base=$$(echo "$$latest" | sed -E 's/^v//; s/-dev\.[0-9]+$$//'); \
	tag="v$$base"; \
	echo "Promoting $$latest → $$tag"; \
	git tag "$$tag" && git push origin "$$tag"

# Direct patch release to prod, skipping dev — must be on a prod tag:
#   v1.2.3 → v1.2.4
release-patch: verify
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	if echo "$$latest" | grep -qE '\-dev\.'; then \
	  echo "Error: on dev tag ($$latest). Use 'release-prod' to promote first, or 'release-patch' only from a prod tag."; \
	  exit 1; \
	fi; \
	base=$$(echo "$$latest" | sed 's/^v//'); \
	major=$$(echo "$$base" | cut -d. -f1); \
	minor=$$(echo "$$base" | cut -d. -f2); \
	patch=$$(echo "$$base" | cut -d. -f3); \
	tag="v$$major.$$minor.$$((patch + 1))"; \
	echo "Tagging: $$tag"; \
	git tag "$$tag" && git push origin "$$tag"

# Direct minor release to prod, skipping dev — must be on a prod tag:
#   v1.2.3 → v1.3.0
release-minor: verify
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	if echo "$$latest" | grep -qE '\-dev\.'; then \
	  echo "Error: on dev tag ($$latest). Use 'release-prod' to promote first."; \
	  exit 1; \
	fi; \
	base=$$(echo "$$latest" | sed 's/^v//'); \
	major=$$(echo "$$base" | cut -d. -f1); \
	minor=$$(echo "$$base" | cut -d. -f2); \
	tag="v$$major.$$((minor + 1)).0"; \
	echo "Tagging: $$tag"; \
	git tag "$$tag" && git push origin "$$tag"
