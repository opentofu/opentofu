# Copilot Instructions for OpenTofu ORAS Fork

## Project Overview

This is a **fork** of [opentofu/opentofu](https://github.com/opentofu/opentofu) that adds an **ORAS backend** for storing OpenTofu state in OCI registries (Docker, GHCR, ECR, etc.).

## Branch Strategy

```
main      → Synchronized with opentofu/opentofu:main (tracking only, do not commit here)
develop   → Main development branch (all PRs target here)
```

### Branch Rules

| Branch | Purpose | Commits |
|--------|---------|---------|
| `main` | Tracks upstream opentofu/opentofu | Only via sync-upstream workflow |
| `develop` | All fork development | Via Pull Requests |

### Workflow

1. **Sync upstream**: `sync-upstream.yml` runs daily and when new upstream tags are detected
2. **PR to develop**: Creates PR `🚀 Release vX.Y.Z` from `main` → `develop`
3. **Review & merge**: Manually merge the PR (resolve conflicts if any)
4. **Auto-release**: `auto-release.yml` creates tag `vX.Y.Z-oci` and GitHub Release
5. **Build**: `release-fork.yml` builds binaries for all platforms

## Release Naming Convention

Fork releases follow upstream versions with `-oci` suffix:

- Upstream: `v1.12.0`
- Fork: `v1.12.0-oci`

This allows users to choose which upstream version they want with ORAS support.

## Key Directories

### ORAS Backend (main contribution)

```
internal/backend/remote-state/oras/
├── backend.go          # Backend implementation
├── client.go           # OCI registry client
├── state.go            # State management
├── locking.go          # Distributed locking
├── versioning.go       # State versioning
├── README.md           # Detailed documentation
└── *_test.go           # Tests
```

### Fork-specific files (not in upstream)

```
.github/
├── copilot-instructions.md    # This file
├── release.yml                # Release notes configuration
├── labeler.yml                # PR auto-labeling rules
└── workflows/
    ├── release-fork.yml       # Fork release workflow
    ├── sync-upstream.yml      # Upstream sync automation
    ├── auto-release.yml       # Auto-tagging on merge
    └── labeler.yml            # PR labeler workflow
```

## Development Guidelines

### Creating PRs

1. Always target `develop` branch
2. Use descriptive titles for release notes generation
3. Apply appropriate labels (auto-labeler will help):
   - `oras`, `oci`, `backend` - ORAS backend changes
   - `enhancement`, `feature` - New features
   - `bug`, `fix` - Bug fixes
   - `documentation` - Docs changes
   - `ci` - CI/CD changes

### Commit Messages

No strict format required, but be descriptive. Examples:
- `Add compression support to ORAS backend`
- `Fix lock acquisition race condition`
- `Update CI workflows for develop branch`

### Testing

```bash
# Run ORAS backend tests
go test ./internal/backend/remote-state/oras/...

# Run all tests
go test ./...
```

## Files to NEVER modify on develop

These files should only change via upstream sync:

- `LICENSE`
- `CHARTER.md`
- `GOVERNANCE.md`
- Core OpenTofu code (unless fixing integration with ORAS)

## Labels for Release Notes

PRs are automatically categorized in releases based on labels:

| Label | Category |
|-------|----------|
| `enhancement`, `feature` | 🚀 Features |
| `bug`, `fix` | 🐛 Bug Fixes |
| `oras`, `oci`, `backend` | 📦 ORAS Backend |
| `security` | 🔒 Security |
| `documentation` | 📚 Documentation |
| `test` | 🧪 Tests |
| `maintenance`, `chore` | 🔧 Maintenance |
