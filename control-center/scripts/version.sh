#!/bin/bash

# Version Management Script
# Usage: ./scripts/version.sh [command] [args]
#
# Commands:
#   bump [major|minor|patch]  - Bump version
#   freeze <version>           - Create frozen version branch
#   release <version>          - Create release
#   current                    - Show current version
#   validate <version>         - Validate version format

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Version file path
VERSION_FILE="${PROJECT_DIR}/VERSION"
CHANGELOG_FILE="${PROJECT_DIR}/CHANGELOG.md"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get current version
get_current_version() {
    if [ -f "$VERSION_FILE" ]; then
        cat "$VERSION_FILE"
    else
        git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"
    fi
}

# Parse version
parse_version() {
    local version=$1
    version=${version#v}
    echo "$version"
}

# Validate version format
validate_version() {
    local version=$1
    if [[ $version =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
        return 0
    else
        return 1
    fi
}

# Bump version
bump_version() {
    local part=$1
    local current=$(get_current_version)
    current=$(parse_version "$current")

    local major minor patch
    IFS='.' read -r major minor patch <<< "$current"

    case $part in
        major)
            major=$((major + 1))
            minor=0
            patch=0
            ;;
        minor)
            minor=$((minor + 1))
            patch=0
            ;;
        patch)
            patch=$((patch + 1))
            ;;
        *)
            echo -e "${RED}Error: Unknown part '$part'. Use: major, minor, or patch${NC}"
            exit 1
            ;;
    esac

    local new_version="v${major}.${minor}.${patch}"
    echo -e "${GREEN}Bumping version: ${current} -> ${new_version}${NC}"

    echo "$new_version" > "$VERSION_FILE"
    echo "Version updated to $new_version"
}

# Freeze version
freeze_version() {
    local version=$1

    if [ -z "$version" ]; then
        echo -e "${RED}Error: Version required${NC}"
        echo "Usage: $0 freeze <version>"
        exit 1
    fi

    # Ensure version starts with 'v'
    if [[ ! $version =~ ^v ]]; then
        version="v${version}"
    fi

    if ! validate_version "$version"; then
        echo -e "${RED}Error: Invalid version format: $version${NC}"
        echo "Expected format: v0.0.0"
        exit 1
    fi

    echo -e "${BLUE}Creating frozen version branch: ${version}${NC}"

    # Check if branch exists
    if git show-ref --verify --quiet "refs/heads/${version}"; then
        echo -e "${YELLOW}Warning: Branch ${version} already exists${NC}"
        exit 1
    fi

    # Run all tests before freezing
    echo "Running tests before freeze..."
    if ! go test ./...; then
        echo -e "${RED}Error: Tests failed. Cannot freeze version.${NC}"
        exit 1
    fi

    # Run critical tests
    echo "Running critical tests..."
    if ! go test -run "TestRegister|TestStartPlugin|TestValidateToken" ./...; then
        echo -e "${RED}Error: Critical tests failed. Cannot freeze version.${NC}"
        exit 1
    fi

    # Create branch
    git checkout -b "${version}"

    # Update VERSION file
    echo "$version" > "$VERSION_FILE"
    git add VERSION
    git commit -m "chore: freeze version ${version}"

    # Create tag
    git tag -a "${version}" -m "Release ${version}"

    echo -e "${GREEN}Version ${version} frozen successfully!${NC}"
    echo ""
    echo -e "${YELLOW}IMPORTANT: Push to origin and set branch protection:${NC}"
    echo "  git push origin ${version}"
    echo "  git push origin ${version} --tags"
    echo ""
    echo "Set branch protection via GitHub UI to prevent any changes."
}

# Create release
create_release() {
    local version=$1

    if [ -z "$version" ]; then
        version=$(get_current_version)
    fi

    if ! validate_version "$version"; then
        echo -e "${RED}Error: Invalid version format: $version${NC}"
        exit 1
    fi

    echo -e "${BLUE}Creating release: ${version}${NC}"

    # Build all platforms
    echo "Building all platforms..."
    make build-release VERSION="$version"

    # Generate checksums
    echo "Generating checksums..."
    make package-release VERSION="$version"

    # Create git tag
    git tag -a "${version}" -m "Release ${version}"

    echo -e "${GREEN}Release ${version} created!${NC}"
    echo ""
    echo "Push to origin:"
    echo "  git push origin ${version} --tags"
}

# Show current version
show_current() {
    local version=$(get_current_version)
    local commit=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    local branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

    echo -e "${BLUE}AnixOps Control Center${NC}"
    echo -e "  Version: ${GREEN}${version}${NC}"
    echo -e "  Commit:  ${commit}"
    echo -e "  Branch:  ${branch}"
}

# Main
case "${1:-}" in
    bump)
        bump_version "${2:-patch}"
        ;;
    freeze)
        freeze_version "$2"
        ;;
    release)
        create_release "$2"
        ;;
    current)
        show_current
        ;;
    validate)
        if validate_version "$2"; then
            echo -e "${GREEN}Valid version: $2${NC}"
        else
            echo -e "${RED}Invalid version: $2${NC}"
            exit 1
        fi
        ;;
    *)
        echo "AnixOps Control Center - Version Management"
        echo ""
        echo "Usage: $0 <command> [args]"
        echo ""
        echo "Commands:"
        echo "  bump [major|minor|patch]  Bump version"
        echo "  freeze <version>          Create frozen version branch"
        echo "  release [version]         Create release"
        echo "  current                   Show current version"
        echo "  validate <version>        Validate version format"
        ;;
esac