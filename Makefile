# Movie Night Showdown — make targets
#
# Pass arguments to the publisher via ARGS, e.g.:
#   make publish ARGS="minor"
#   make publish ARGS="0.2.0 --allow-dirty"
#   make publish-dry ARGS="major"

.PHONY: publish publish-dry

# Build the Docker image, push it to the registry, and tag the release.
# See scripts/publish.sh for the version model and flags.
publish:
	./scripts/publish.sh $(ARGS)

# Preview a publish (prints the plan and commands) without building,
# pushing, or tagging anything.
publish-dry:
	./scripts/publish.sh --dry-run $(ARGS)
