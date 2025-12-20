#!/usr/bin/env bash
# update-pr.sh - Keep backend/oci PR branch rebased on upstream

set -euo pipefail

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info()    { echo -e "${BLUE}ℹ${NC} $*"; }
log_success() { echo -e "${GREEN}✓${NC} $*"; }
log_warn()    { echo -e "${YELLOW}⚠${NC} $*"; }
log_error()   { echo -e "${RED}✗${NC} $*"; }

usage() {
  cat <<'EOF'
Usage: ./update-pr.sh [options]

What it does (default):
  1) Fast-forward main to upstream/main
  2) Rebase backend/oci on top of main
  3) Push backend/oci with --force-with-lease

Options:
  --no-push        Do not push anything (dry run for safety)
  --skip-main      Do not update main (only rebase backend/oci)
  --skip-fetch     Do not fetch remotes (assume you already did)
  --help           Show this help

Assumptions:
  - You have remotes: origin (your fork) and upstream (opentofu/opentofu)
  - Branches: main and backend/oci
EOF
}

NO_PUSH=false
SKIP_MAIN=false
SKIP_FETCH=false

while [ $# -gt 0 ]; do
  case "$1" in
    --no-push) NO_PUSH=true ;;
    --skip-main) SKIP_MAIN=true ;;
    --skip-fetch) SKIP_FETCH=true ;;
    -h|--help) usage; exit 0 ;;
    *) log_error "Unknown option: $1"; usage; exit 1 ;;
  esac
  shift
done

# Must run from repo root (or inside repo)
if ! git rev-parse --git-dir >/dev/null 2>&1; then
  log_error "Not inside a git repository"
  exit 1
fi

# Ensure clean working tree
if ! git diff-index --quiet HEAD --; then
  log_error "Working directory has uncommitted changes"
  log_info "Commit or stash changes before running this script"
  exit 1
fi

# Ensure upstream remote
if ! git remote get-url upstream >/dev/null 2>&1; then
  log_warn "Remote 'upstream' not found; adding https://github.com/opentofu/opentofu.git"
  git remote add upstream https://github.com/opentofu/opentofu.git
fi

current_branch=$(git rev-parse --abbrev-ref HEAD)

if [ "$SKIP_FETCH" = false ]; then
  log_info "Fetching origin + upstream..."
  git fetch origin --prune --quiet
  git fetch upstream --prune --quiet
fi

if [ "$SKIP_MAIN" = false ]; then
  log_info "Updating main (fast-forward to upstream/main)..."
  git checkout main --quiet 2>/dev/null || git checkout -b main origin/main --quiet
  git merge --ff-only upstream/main --quiet
  log_success "main is now at $(git rev-parse --short HEAD)"

  if [ "$NO_PUSH" = false ]; then
    log_info "Pushing main to origin..."
    git push origin main --quiet
    log_success "Pushed main"
  else
    log_warn "Skipping push of main (--no-push)"
  fi
else
  log_warn "Skipping main update (--skip-main)"
fi

target_base=main

log_info "Rebasing backend/oci onto ${target_base}..."

git checkout backend/oci --quiet 2>/dev/null || git checkout -b backend/oci origin/backend/oci --quiet

# Rebase; if conflicts, user must resolve and continue.
if git rebase "${target_base}"; then
  log_success "Rebase successful"
else
  log_error "Rebase stopped due to conflicts"
  log_info "Resolve conflicts and then run:"
  echo "  git rebase --continue"
  echo "  git push --force-with-lease origin backend/oci"
  exit 1
fi

if [ "$NO_PUSH" = false ]; then
  log_info "Pushing backend/oci with --force-with-lease..."
  git push --force-with-lease origin backend/oci
  log_success "Pushed backend/oci"
else
  log_warn "Skipping push of backend/oci (--no-push)"
fi

# Return to previous branch if it still exists
if [ "$current_branch" != "backend/oci" ]; then
  git checkout "$current_branch" --quiet 2>/dev/null || true
fi

log_success "Done"
