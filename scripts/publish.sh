#!/usr/bin/env bash
#
# publish.sh
# Builds the movie-night-showdown docker image (amd64), tags it with the
# next semver, pushes both tags to the registry, and (only on a successful
# push) creates and pushes the matching git tag.
#
# Version model: git tags are the source of truth. The version is baked
# into the binary at build time via -ldflags (see Dockerfile) and reported
# at /healthz.
#
# Usage:
#   ./scripts/publish.sh [major|minor|patch|<X.Y.Z>] [--allow-dirty] [--dry-run]
#
# Examples:
#   ./scripts/publish.sh                # Increments patch (e.g., 1.1.1 -> 1.1.2)
#   ./scripts/publish.sh minor          # Increments minor (e.g., 1.1.1 -> 1.2.0)
#   ./scripts/publish.sh major          # Increments major (e.g., 1.1.1 -> 2.0.0)
#   ./scripts/publish.sh 0.1.5          # Sets exact version
#   ./scripts/publish.sh --dry-run      # Show what would happen, do nothing
#   ./scripts/publish.sh --allow-dirty  # Publish despite tracked local changes
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "${SCRIPT_DIR}")"

cd "${PROJECT_ROOT}"

REGISTRY="registry.eiladin.xyz"
IMAGE="movie-night-showdown"
IMAGE_BASE="${REGISTRY}/${IMAGE}"

usage() {
    echo -e "${BLUE}Movie Night Showdown Publisher${NC}"
    echo ""
    echo "Usage: $0 [major|minor|patch|<X.Y.Z>] [--allow-dirty] [--dry-run]"
    echo "  major         Increment major version and reset minor/patch to 0 (e.g., 1.1.1 -> 2.0.0)"
    echo "  minor         Increment minor version and reset patch to 0 (e.g., 1.1.1 -> 1.2.0)"
    echo "  patch         Increment patch version (default, e.g., 1.1.1 -> 1.1.2)"
    echo "  <X.Y.Z>       Explicitly set a version (e.g., 0.1.5)"
    echo "  --allow-dirty Publish even if the working tree has tracked changes"
    echo "  --dry-run     Print the plan and exact commands without running them"
    echo ""
    echo "Note: Make sure you are logged in to the registry first:"
    echo "  docker login ${REGISTRY}"
    exit 1
}

MODE="patch"
ALLOW_DIRTY=0
DRY_RUN=0
MODE_SET=0

for arg in "$@"; do
    case "${arg}" in
        -h|--help|help)
            usage
            ;;
        --allow-dirty)
            ALLOW_DIRTY=1
            ;;
        --dry-run)
            DRY_RUN=1
            ;;
        major|minor|patch)
            MODE="${arg}"
            MODE_SET=1
            ;;
        [0-9]*.[0-9]*.[0-9]*)
            if [[ ! "${arg}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
                echo -e "${RED}Error: Invalid version '${arg}'. Expected X.Y.Z.${NC}"
                usage
            fi
            MODE="${arg}"
            MODE_SET=1
            ;;
        *)
            echo -e "${RED}Error: Invalid argument '${arg}'.${NC}"
            usage
            ;;
    esac
done

if [[ "${MODE_SET}" -eq 0 ]]; then
    MODE="patch"
fi

# 1. Determine CURRENT version from git tags (source of truth).
BOOTSTRAP=0
if CURRENT_TAG=$(git describe --tags --abbrev=0 2>/dev/null); then
    CURRENT_VERSION="${CURRENT_TAG#v}"
else
    BOOTSTRAP=1
    CURRENT_VERSION="0.0.0"
fi

# 2. Compute TARGET version.
if [[ "${MODE}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    # Explicit version always wins, bootstrap or not.
    TARGET="${MODE}"
elif [[ "${BOOTSTRAP}" -eq 1 ]]; then
    TARGET="0.1.0"
else
    IFS='.' read -r MAJOR MINOR PATCH <<< "${CURRENT_VERSION}"
    case "${MODE}" in
        patch)
            TARGET="${MAJOR}.${MINOR}.$((PATCH + 1))"
            ;;
        minor)
            TARGET="${MAJOR}.$((MINOR + 1)).0"
            ;;
        major)
            TARGET="$((MAJOR + 1)).0.0"
            ;;
        *)
            echo -e "${RED}Error: Invalid argument '${MODE}'.${NC}"
            usage
            ;;
    esac
fi

if [[ "${BOOTSTRAP}" -eq 1 ]]; then
    echo -e "${YELLOW}Note: no git tags found. Bootstrapping first release.${NC}"
fi

# 3. Refuse to publish with tracked local changes, unless --allow-dirty.
if [[ "${ALLOW_DIRTY}" -eq 0 ]] && [[ -n "$(git status --porcelain --untracked-files=no)" ]]; then
    if [[ "${DRY_RUN}" -eq 1 ]]; then
        echo -e "${YELLOW}Note: working tree has tracked changes. A real run would refuse unless --allow-dirty is passed.${NC}"
    else
        echo -e "${RED}Error: working tree has tracked changes.${NC}"
        echo -e "${YELLOW}Commit or stash them, or re-run with --allow-dirty.${NC}"
        exit 1
    fi
fi

# 4. Guard: target git tag must not already exist.
GIT_TAG="v${TARGET}"
if git rev-parse -q --verify "refs/tags/${GIT_TAG}" >/dev/null; then
    if [[ "${DRY_RUN}" -eq 1 ]]; then
        echo -e "${YELLOW}Note: git tag ${GIT_TAG} already exists. A real run would refuse to publish.${NC}"
    else
        echo -e "${RED}Error: git tag ${GIT_TAG} already exists.${NC}"
        exit 1
    fi
fi

COMMIT=$(git rev-parse --short HEAD)

echo -e "${BLUE}=== Publish plan ===${NC}"
echo -e "  Current version: ${CURRENT_VERSION}$([[ ${BOOTSTRAP} -eq 1 ]] && echo ' (no tags yet)')"
echo -e "  Target version:  ${TARGET} (mode: ${MODE})"
echo -e "  Commit:          ${COMMIT}"
echo -e "  Image tags:      ${IMAGE_BASE}:${TARGET}"
echo -e "                    ${IMAGE_BASE}:latest"
echo -e "  Git tag:         ${GIT_TAG}"
echo ""

BUILD_CMD=(docker build --build-arg "VERSION=${TARGET}" --build-arg "COMMIT=${COMMIT}" -t "${IMAGE_BASE}:${TARGET}" -t "${IMAGE_BASE}:latest" -f Dockerfile .)
PUSH_VERSION_CMD=(docker push "${IMAGE_BASE}:${TARGET}")
PUSH_LATEST_CMD=(docker push "${IMAGE_BASE}:latest")
TAG_CMD=(git tag -a "${GIT_TAG}" -m "release ${GIT_TAG}")
PUSH_TAG_CMD=(git push origin "${GIT_TAG}")

if [[ "${DRY_RUN}" -eq 1 ]]; then
    echo -e "${YELLOW}=== Dry run: no commands will be executed ===${NC}"
    echo "Would run:"
    echo "  ${BUILD_CMD[*]}"
    echo "  ${PUSH_VERSION_CMD[*]}"
    echo "  ${PUSH_LATEST_CMD[*]}"
    echo "  ${TAG_CMD[*]}"
    echo "  ${PUSH_TAG_CMD[*]}"
    echo ""
    echo -e "${GREEN}Dry run complete. Nothing was built, pushed, or tagged.${NC}"
    exit 0
fi

# 5. Build (amd64).
echo -e "${BLUE}=== Building ${IMAGE} image (amd64) ===${NC}"
"${BUILD_CMD[@]}"

# 6. Push both tags.
echo -e "${BLUE}=== Pushing to registry ${REGISTRY} ===${NC}"
if ! "${PUSH_VERSION_CMD[@]}"; then
    echo -e "${RED}Error: Failed to push ${IMAGE_BASE}:${TARGET}. Are you logged in?${NC}"
    echo -e "${YELLOW}Run: docker login ${REGISTRY}${NC}"
    exit 1
fi
if ! "${PUSH_LATEST_CMD[@]}"; then
    echo -e "${RED}Error: Failed to push ${IMAGE_BASE}:latest. Are you logged in?${NC}"
    echo -e "${YELLOW}Run: docker login ${REGISTRY}${NC}"
    exit 1
fi

echo -e "${GREEN}Image successfully pushed!${NC}"

# 7. Only after a successful push: create and push the git tag.
echo -e "${BLUE}=== Tagging release ${GIT_TAG} ===${NC}"
"${TAG_CMD[@]}"

if ! "${PUSH_TAG_CMD[@]}"; then
    echo -e "${YELLOW}Warning: failed to push git tag ${GIT_TAG} to origin.${NC}"
    echo -e "${YELLOW}The image is already published; push the tag manually: git push origin ${GIT_TAG}${NC}"
fi

echo ""
echo -e "${GREEN}=== Build and publish complete! ===${NC}"
echo "image: ${IMAGE_BASE}:${TARGET}"
echo -e "${YELLOW}Remember to bump this tag in your deploy stack.${NC}"
