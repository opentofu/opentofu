#!/bin/bash
# create-oci-release.sh - Create OCI fork release based on upstream version

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

log_success() {
    echo -e "${GREEN}✓${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

log_error() {
    echo -e "${RED}✗${NC} $1"
}

# Check arguments
if [ $# -lt 1 ]; then
    cat << 'EOF'
Usage: ./create-oci-release.sh <upstream-version> [--force] [--no-push]

Examples:
  ./create-oci-release.sh v1.12.0
  ./create-oci-release.sh v1.12.0 --no-push
  ./create-oci-release.sh v1.12.0 --force

Options:
  --force     Force-push if branch already exists
  --no-push   Don't push to remote (useful for testing)

Environment:
  GITHUB_TOKEN (optional) - For pushing to private repos
EOF
    exit 1
fi

UPSTREAM_VERSION=$1
OCI_VERSION="${UPSTREAM_VERSION}-oci"
FORCE_PUSH=false
NO_PUSH=false

# Parse additional arguments
shift || true
while [ $# -gt 0 ]; do
    case "$1" in
        --force)
            FORCE_PUSH=true
            ;;
        --no-push)
            NO_PUSH=true
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
    shift || true
done

# Validate version format
if ! [[ $UPSTREAM_VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
    log_error "Invalid version format: $UPSTREAM_VERSION"
    log_info "Expected format: v1.12.0 or v1.12.0-rc1"
    exit 1
fi

log_info "OpenTofu OCI Fork Release Script"
log_info "Upstream version: ${UPSTREAM_VERSION}"
log_info "Release version: ${OCI_VERSION}"
echo ""

# Check git status
if ! git diff-index --quiet HEAD --; then
    log_error "Working directory has uncommitted changes"
    log_info "Please commit or stash changes before running this script"
    exit 1
fi

# Ensure we have upstream remote configured
if ! git remote get-url upstream &>/dev/null; then
    log_info "Adding upstream remote..."
    git remote add upstream https://github.com/opentofu/opentofu.git
fi

# Fetch latest tags
log_info "Fetching latest tags from upstream..."
git fetch upstream --tags --quiet

# Check if upstream version exists
if ! git rev-parse "${UPSTREAM_VERSION}" &>/dev/null; then
    log_error "Upstream version ${UPSTREAM_VERSION} not found"
    log_info "Available versions:"
    git tag -l 'v*' --sort=-version:refname | head -10
    exit 1
fi

log_success "Upstream version ${UPSTREAM_VERSION} found"
echo ""

# Sync main branch
log_info "Step 1: Syncing main branch with upstream..."
git checkout main --quiet 2>/dev/null || git checkout -b main origin/main --quiet
git fetch origin main --quiet
git merge upstream/main --ff-only --quiet 2>/dev/null || true
if [ "$NO_PUSH" = false ]; then
    git push origin main --quiet
fi
log_success "Main branch synced"
echo ""

# Update or create oci-releases branch
log_info "Step 2: Updating oci-releases branch..."

if git rev-parse origin/oci-releases --verify &>/dev/null; then
    log_info "oci-releases branch exists, updating..."
    git checkout oci-releases --quiet 2>/dev/null || git checkout -b oci-releases origin/oci-releases --quiet
else
    log_info "Creating oci-releases branch from backend/oci..."
    git checkout oci-releases --quiet 2>/dev/null || git checkout -b oci-releases backend/oci --quiet
fi

# Merge upstream version
log_info "Merging upstream ${UPSTREAM_VERSION}..."
if git merge ${UPSTREAM_VERSION} --no-ff -m "Merge upstream ${UPSTREAM_VERSION}" --quiet; then
    log_success "Merge successful"
    log_info "Downloading Go modules..."
    go mod download
    log_success "Go modules downloaded"
else
    log_warn "Merge conflicts detected"
    echo ""
    log_info "Please resolve the conflicts and run:"
    echo "    git add ."
    echo "    git commit -m 'Merge upstream ${UPSTREAM_VERSION}'"
    echo "    git tag -a ${OCI_VERSION} -m 'Release ${OCI_VERSION} with OCI backend'"
    if [ "$NO_PUSH" = false ]; then
        echo "    git push origin oci-releases ${OCI_VERSION}"
    fi
    exit 1
fi
echo ""

# Create tag
log_info "Step 3: Creating release tag..."

if git rev-parse "${OCI_VERSION}" &>/dev/null; then
    log_warn "Tag ${OCI_VERSION} already exists"
    if [ "$FORCE_PUSH" = true ]; then
        log_info "Removing existing tag..."
        git tag -d ${OCI_VERSION}
    else
        log_error "Use --force to overwrite existing tag"
        exit 1
    fi
fi

git tag -a ${OCI_VERSION} -m "Release ${OCI_VERSION} with OCI backend

Based on upstream: ${UPSTREAM_VERSION}

This release includes the OpenTofu OCI registry backend for state storage.
See: rfc/20251220-oci-state-backend-design.md"

log_success "Tag ${OCI_VERSION} created"
echo ""

# Push to remote
if [ "$NO_PUSH" = true ]; then
    log_warn "Skipping push (--no-push flag)"
    echo ""
    log_info "To push manually, run:"
    echo "    git push origin oci-releases ${OCI_VERSION}"
else
    log_info "Step 4: Pushing to remote..."
    
    if [ "$FORCE_PUSH" = true ]; then
        git push origin oci-releases --force-with-lease --quiet
    else
        git push origin oci-releases --quiet
    fi
    
    git push origin ${OCI_VERSION} --quiet
    log_success "Pushed to remote"
    echo ""
fi

# Summary
echo ""
log_success "Release preparation complete!"
echo ""
log_info "Release details:"
echo "  Tag: ${OCI_VERSION}"
echo "  Branch: oci-releases"
echo "  Based on: ${UPSTREAM_VERSION}"
echo ""

if [ "$NO_PUSH" = false ]; then
    log_info "GitHub Actions will now:"
    echo "  1. Build binaries for all platforms"
    echo "  2. Generate checksums"
    echo "  3. Create GitHub release with assets"
    echo ""
    log_info "Monitor build progress:"
    REPO=$(git remote get-url origin | sed 's/.*://;s/.git$//')
    echo "  https://github.com/${REPO}/actions"
    echo ""
    log_info "Release will be available at:"
    echo "  https://github.com/${REPO}/releases/tag/${OCI_VERSION}"
else
    log_info "Build process skipped. To trigger the release:"
    echo "    git push origin oci-releases ${OCI_VERSION}"
fi

echo ""
