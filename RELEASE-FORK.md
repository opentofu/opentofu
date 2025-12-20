# OCI Fork Release Infrastructure

## Overview

This document describes the release infrastructure for the OpenTofu OCI fork.

## Branch Structure

```
├── main
│   ├─ Synced with upstream/main
│   ├─ No permanent workflows here
│   └─ Used for base tracking only
│
├── backend/oci
│   ├─ The actual backend implementation PR
│   ├─ Rebased on latest upstream
│   └─ Target for upstream pull request
│
└── oci-releases (permanent release branch)
    ├─ Base: backend/oci + latest upstream version
    ├─ Contains: release-fork.yml workflow
    ├─ Source of: version tags (v*.*.* +oci)
    └─ Generates: GitHub releases with assets
```

## Release Workflow

### Automated Process

When you push a tag `v*.*.*.+oci` to `oci-releases`, GitHub Actions:

1. **Disk space preparation** - Frees up 15+ GB on runner
2. **Parallel builds** - Builds 8 platform variants simultaneously
   - Linux: amd64, arm64, arm, 386
   - macOS: amd64, arm64
   - Windows: amd64, arm64
3. **Artifact collection** - Downloads all binaries
4. **Release creation** - Creates GitHub release with:
   - All platform binaries
   - SHA256SUMS file
   - Detailed release notes
   - Installation instructions

### Creating a Release

#### Quick Method (Recommended)

```bash
./create-oci-release.sh v1.12.0
```

The script will:
- ✓ Sync main with upstream
- ✓ Update oci-releases branch
- ✓ Merge upstream version
- ✓ Create tag v1.12.0+oci
- ✓ Push to GitHub
- ✓ Trigger GitHub Actions build

#### Manual Method

```bash
# Fetch upstream tags
git fetch upstream --tags

# Switch to oci-releases
git checkout oci-releases

# Merge upstream version
git merge v1.12.0 --no-ff -m "Merge upstream v1.12.0"

# Create release tag
git tag -a v1.12.0+oci -m "Release v1.12.0 with OCI backend"

# Push (triggers GitHub Actions)
git push origin oci-releases v1.12.0+oci
```

## Script Options

### create-oci-release.sh

```bash
# Normal release
./create-oci-release.sh v1.12.0

# Without pushing (test locally)
./create-oci-release.sh v1.12.0 --no-push

# Force push if tag exists
./create-oci-release.sh v1.12.0 --force

# Combine options
./create-oci-release.sh v1.12.0 --force --no-push
```

## Workflow Features

### release-fork.yml

**Triggers:** Push of tag matching `v*+oci`

**Jobs:**

1. **prepare** - Free disk space
2. **build** - 8 parallel matrix jobs
   - Each builds one platform variant
   - Uploads artifacts with 1-day retention
3. **create-release** - Create GitHub release
   - Downloads all artifacts
   - Generates SHA256SUMS
   - Creates release with full notes

**Optimization strategies:**

- Remove dotnet, Android SDK, CodeQL
- Clean Docker images
- Clean apt cache
- Per-platform builds (no monolithic build)

## Troubleshooting

### Merge Conflicts

If `merge upstream` fails:

```bash
git checkout oci-releases
git merge v1.12.0

# Resolve conflicts in your editor
git add .
git commit -m "Merge upstream v1.12.0"
git tag -a v1.12.0+oci -m "Release v1.12.0 with OCI backend"
git push origin oci-releases v1.12.0+oci
```

### Build Failures

Check the Actions log:
```
https://github.com/vmvarela/opentofu/actions
```

Common issues:
- **Disk space** - Workflow handles cleanup, but can still fail on large builds
- **Platform-specific** - Some platforms may need special handling
- **Credential issues** - Verify GitHub token permissions

### Disk Space Issues

If `ubuntu-latest` still runs out of space:

1. Reduce matrix (e.g., remove 386 and arm)
2. Split into multiple workflows by platform
3. Use self-hosted runner with more storage

Current setup should handle all 8 platforms within 20GB limit.

## Release Contents

Each release includes:

```
tofu_linux_amd64              # Linux AMD64 binary
tofu_linux_amd64.sha256      # Checksum
tofu_linux_arm64             # Linux ARM64 binary
tofu_linux_arm64.sha256      # Checksum
tofu_linux_arm               # Linux ARM binary
tofu_linux_arm.sha256        # Checksum
tofu_linux_386               # Linux 386 binary
tofu_linux_386.sha256        # Checksum
tofu_darwin_amd64            # macOS Intel binary
tofu_darwin_amd64.sha256     # Checksum
tofu_darwin_arm64            # macOS ARM binary
tofu_darwin_arm64.sha256     # Checksum
tofu_windows_amd64.exe       # Windows AMD64 binary
tofu_windows_amd64.exe.sha256 # Checksum
tofu_windows_arm64.exe       # Windows ARM64 binary
tofu_windows_arm64.exe.sha256 # Checksum
SHA256SUMS                    # All checksums
```

## Verifying Releases

```bash
# Download release and checksums
wget https://github.com/vmvarela/opentofu/releases/download/v1.12.0+oci/tofu_linux_amd64
wget https://github.com/vmvarela/opentofu/releases/download/v1.12.0+oci/SHA256SUMS

# Verify
sha256sum -c SHA256SUMS
```

## Maintenance

### Branch Maintenance

The `oci-releases` branch should:
- Always have latest upstream version merged
- Include all OCI backend commits
- Have release workflow setup

### Tag Cleanup

If you need to delete a release:

```bash
# Delete local tag
git tag -d v1.12.0+oci

# Delete remote tag
git push origin --delete v1.12.0+oci

# Delete GitHub release (via web UI)
```

## Integration with Upstream

When you update `backend/oci` with fixes:

1. Push to `backend/oci` (for PR)
2. Manually update `oci-releases`:
   ```bash
   git checkout oci-releases
   git merge backend/oci
   git push origin oci-releases
   ```

3. Next release will include those fixes

## Future Enhancements

Potential improvements:

- [ ] Automated upstream detection (email/webhook)
- [ ] Automatic PR creation on new upstream release
- [ ] Checksum signing with GPG
- [ ] SBOM generation
- [ ] Docker image builds
- [ ] Brew formula updates
