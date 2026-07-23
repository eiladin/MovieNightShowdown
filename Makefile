# Movie Night Showdown — make targets
#
# Publish targets:
#   make publish              Increment patch (default) and release
#   make publish-minor        Increment minor and release
#   make publish-major        Increment major and release
#   make publish-patch        Increment patch and release (explicit)
#   make publish-dry          Preview a patch release, do nothing
#
# Extra flags / an explicit version go through ARGS, e.g.:
#   make publish ARGS="0.2.0 --allow-dirty"
#   make publish-minor ARGS="--allow-dirty"
#   make publish-dry ARGS="major"

.PHONY: publish publish-patch publish-minor publish-major publish-dry

# Build the Docker image, push it to the registry, and tag the release.
# See scripts/publish.sh for the version model and flags.
publish:
	./scripts/publish.sh $(ARGS)

publish-patch:
	./scripts/publish.sh patch $(ARGS)

publish-minor:
	./scripts/publish.sh minor $(ARGS)

publish-major:
	./scripts/publish.sh major $(ARGS)

# Preview a publish (prints the plan and commands) without building,
# pushing, or tagging anything.
publish-dry:
	./scripts/publish.sh --dry-run $(ARGS)
