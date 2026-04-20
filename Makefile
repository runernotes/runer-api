.PHONY: lint test e2e build dev docker-build docker-run verify \
        version release-dev release-minor-dev release-prod release-patch release-minor \
        check-changelog changelog-version changelog-prod changelog-patch changelog-minor changelog-major

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
# Changelog helpers — insert the next version section into CHANGELOG.md.
# Each target mirrors its release counterpart exactly. Run changelog-version
# to preview what each target would insert given the current tag.
#
#   Target              From prod v1.2.3        From dev v1.2.3-dev.1
#   ──────────────────  ──────────────────────  ──────────────────────
#   changelog-prod      ✗ error                 → inserts [1.2.3]
#   changelog-patch     → inserts [1.2.4]       ✗ error
#   changelog-minor     → inserts [1.3.0]       ✗ error
#   changelog-major     → inserts [2.0.0]       ✗ error
#
# Typical flow:
#   changelog-prod / changelog-patch / changelog-minor → fill in CHANGELOG.md
#   → matching release target
# ---------------------------------------------------------------------------

# Print the current tag and preview what each changelog target would insert.
changelog-version:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	base=$$(echo "$$latest" | sed -E 's/^v//; s/-dev\.[0-9]+$$//'); \
	maj=$$(echo "$$base" | cut -d. -f1); \
	min=$$(echo "$$base" | cut -d. -f2); \
	pat=$$(echo "$$base" | cut -d. -f3); \
	is_dev=$$(echo "$$latest" | grep -qE '\-dev\.' && echo 1 || echo 0); \
	echo ""; \
	echo "  Current tag: $$latest"; \
	echo ""; \
	echo "  Target              Inserts into CHANGELOG.md"; \
	echo "  ──────────────────  ──────────────────────────"; \
	if [ "$$is_dev" = "1" ]; then \
	  echo "  changelog-prod      [$$base]"; \
	  echo "  changelog-patch     (error — on dev tag, use changelog-prod first)"; \
	  echo "  changelog-minor     (error — on dev tag, use changelog-prod first)"; \
	  echo "  changelog-major     (error — on dev tag, use changelog-prod first)"; \
	else \
	  echo "  changelog-prod      (error — nothing to promote)"; \
	  echo "  changelog-patch     [$$maj.$$min.$$((pat + 1))]"; \
	  echo "  changelog-minor     [$$maj.$$((min + 1)).0]"; \
	  echo "  changelog-major     [$$((maj + 1)).0.0]"; \
	fi; \
	echo ""

# Mirrors release-prod: must be on a dev tag — promotes base version (e.g. v0.4.3-dev.2 → 0.4.3)
changelog-prod:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	if ! echo "$$latest" | grep -qE '\-dev\.'; then \
	  echo "Error: not on a dev tag ($$latest). Nothing to promote."; \
	  exit 1; \
	fi; \
	ver=$$(echo "$$latest" | sed -E 's/^v//; s/-dev\.[0-9]+$$//'); \
	date=$$(date +%Y-%m-%d); \
	awk -v ver="$$ver" -v date="$$date" \
	  '/and this project adheres/{print; print ""; print "## [" ver "] - " date; print "### Added"; print ""; print "### Fixed"; print ""; print "### Changed"; print ""; next} {print}' \
	  CHANGELOG.md > CHANGELOG.tmp && mv CHANGELOG.tmp CHANGELOG.md; \
	echo "Added [$$ver] - $$date to CHANGELOG.md — fill in the details then run: make release-prod"

# Mirrors release-patch: must be on a prod tag
changelog-patch:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	if echo "$$latest" | grep -qE '\-dev\.'; then \
	  echo "Error: on dev tag ($$latest). Use 'make changelog-prod' to prepare notes for the promotion to prod first."; \
	  exit 1; \
	fi; \
	base=$$(echo "$$latest" | sed 's/^v//'); \
	maj=$$(echo "$$base" | cut -d. -f1); \
	min=$$(echo "$$base" | cut -d. -f2); \
	pat=$$(echo "$$base" | cut -d. -f3); \
	ver="$$maj.$$min.$$((pat + 1))"; \
	date=$$(date +%Y-%m-%d); \
	awk -v ver="$$ver" -v date="$$date" \
	  '/and this project adheres/{print; print ""; print "## [" ver "] - " date; print "### Added"; print ""; print "### Fixed"; print ""; print "### Changed"; print ""; next} {print}' \
	  CHANGELOG.md > CHANGELOG.tmp && mv CHANGELOG.tmp CHANGELOG.md; \
	echo "Added [$$ver] - $$date to CHANGELOG.md — fill in the details then run: make release-patch"

# Mirrors release-minor: must be on a prod tag
changelog-minor:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	if echo "$$latest" | grep -qE '\-dev\.'; then \
	  echo "Error: on dev tag ($$latest). Use 'make changelog-prod' to prepare notes for the promotion to prod first."; \
	  exit 1; \
	fi; \
	base=$$(echo "$$latest" | sed 's/^v//'); \
	maj=$$(echo "$$base" | cut -d. -f1); \
	min=$$(echo "$$base" | cut -d. -f2); \
	ver="$$maj.$$((min + 1)).0"; \
	date=$$(date +%Y-%m-%d); \
	awk -v ver="$$ver" -v date="$$date" \
	  '/and this project adheres/{print; print ""; print "## [" ver "] - " date; print "### Added"; print ""; print "### Fixed"; print ""; print "### Changed"; print ""; next} {print}' \
	  CHANGELOG.md > CHANGELOG.tmp && mv CHANGELOG.tmp CHANGELOG.md; \
	echo "Added [$$ver] - $$date to CHANGELOG.md — fill in the details then run: make release-minor"

# Mirrors release-major: must be on a prod tag
changelog-major:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	if echo "$$latest" | grep -qE '\-dev\.'; then \
	  echo "Error: on dev tag ($$latest). Use 'make changelog-prod' to prepare notes for the promotion to prod first."; \
	  exit 1; \
	fi; \
	base=$$(echo "$$latest" | sed 's/^v//'); \
	maj=$$(echo "$$base" | cut -d. -f1); \
	ver="$$((maj + 1)).0.0"; \
	date=$$(date +%Y-%m-%d); \
	awk -v ver="$$ver" -v date="$$date" \
	  '/and this project adheres/{print; print ""; print "## [" ver "] - " date; print "### Added"; print ""; print "### Fixed"; print ""; print "### Changed"; print ""; next} {print}' \
	  CHANGELOG.md > CHANGELOG.tmp && mv CHANGELOG.tmp CHANGELOG.md; \
	echo "Added [$$ver] - $$date to CHANGELOG.md — fill in the details then run: make release-major"

# Internal: confirm CHANGELOG.md has an entry for VERSION (semver without 'v').
# Call as: $(MAKE) check-changelog VERSION=1.2.3
check-changelog:
	@if ! grep -qE "^## \[$(VERSION)\]" CHANGELOG.md; then \
	  echo ""; \
	  echo "  Error: CHANGELOG.md has no entry for [$(VERSION)]."; \
	  echo "  Add a '## [$(VERSION)] - YYYY-MM-DD' section before tagging."; \
	  echo ""; \
	  exit 1; \
	fi; \
	echo "  CHANGELOG.md entry for [$(VERSION)] found."

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
	$(MAKE) --no-print-directory check-changelog VERSION="$$base"; \
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
	next="$$major.$$minor.$$((patch + 1))"; \
	$(MAKE) --no-print-directory check-changelog VERSION="$$next"; \
	tag="v$$next"; \
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
	next="$$major.$$((minor + 1)).0"; \
	$(MAKE) --no-print-directory check-changelog VERSION="$$next"; \
	tag="v$$next"; \
	echo "Tagging: $$tag"; \
	git tag "$$tag" && git push origin "$$tag"
