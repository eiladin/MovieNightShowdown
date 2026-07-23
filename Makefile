# Movie Night Showdown — make targets
#
# Run `make` (or `make help`) to list targets. Extra flags / an explicit
# version go through ARGS, e.g.:
#   make publish ARGS="0.2.0 --allow-dirty"
#   make publish-minor ARGS="--allow-dirty"
#   make publish-dry ARGS="major"

.PHONY: help publish publish-patch publish-minor publish-major publish-dry posters screenshots

.DEFAULT_GOAL := help

help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m\t %s\n", $$1, $$2}'

# Build the Docker image, push it to the registry, and tag the release.
# See scripts/publish.sh for the version model and flags.
publish: ## Build, push, and tag a release (patch bump by default; version/flags via ARGS)
	./scripts/publish.sh $(ARGS)

publish-patch: ## Increment the patch version and release
	./scripts/publish.sh patch $(ARGS)

publish-minor: ## Increment the minor version and release
	./scripts/publish.sh minor $(ARGS)

publish-major: ## Increment the major version and release
	./scripts/publish.sh major $(ARGS)

# Preview a publish (prints the plan and commands) without building,
# pushing, or tagging anything.
publish-dry: ## Preview a patch release without building, pushing, or tagging
	./scripts/publish.sh --dry-run $(ARGS)

# Regenerate the placeholder poster art served by the mock Jellyfin. Run this
# after editing scripts/screenshots/gen-posters.mjs; then run `make screenshots`
# to pull the new art into docs/screenshots.
posters: ## Regenerate the placeholder poster art (scripts/screenshots/fixtures)
	(cd scripts/screenshots && npm install --no-audit --no-fund && npx playwright install chromium)
	node scripts/screenshots/gen-posters.mjs

# Regenerate the README screenshots against a self-contained mock Jellyfin
# server (no real Jellyfin instance or personal data involved).
# See scripts/screenshots/README.md.
screenshots: ## Regenerate docs/screenshots against a self-contained mock Jellyfin
	bash scripts/screenshots/run.sh
