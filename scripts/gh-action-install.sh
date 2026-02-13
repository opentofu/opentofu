#!/usr/bin/env bash
# Setup Ghoten for GitHub Actions
# This script downloads and installs Ghoten to the GitHub Actions tool cache
#
# Environment variables:
#   INPUT_GHOTEN_VERSION - Version to install (without 'v' prefix)
#   INPUT_ALIAS          - Optional alias to create (e.g., 'terraform')
#   CACHE_HIT            - 'true' if binary was restored from cache
#   GITHUB_TOKEN         - GitHub token for API requests

set -euo pipefail

# Configuration
GITHUB_REPO="vmvarela/opentofu"
BINARY_NAME="ghoten"

# Colors (only if running in terminal)
if [[ -t 1 ]]; then
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  NC='\033[0m'
else
  RED=''
  GREEN=''
  YELLOW=''
  NC=''
fi

info() {
  echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
  echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
  echo -e "${RED}[ERROR]${NC} $1"
  exit 1
}

# Map GitHub Actions runner OS to ghoten OS name
map_os() {
  local runner_os="${RUNNER_OS:-}"
  case "$(echo "$runner_os" | tr '[:upper:]' '[:lower:]')" in
    linux)   echo "linux" ;;
    macos)   echo "darwin" ;;
    windows) echo "windows" ;;
    *)       error "Unsupported operating system: ${runner_os}" ;;
  esac
}

# Map GitHub Actions runner architecture to ghoten arch name
map_arch() {
  local runner_arch="${RUNNER_ARCH:-}"
  case "$(echo "$runner_arch" | tr '[:upper:]' '[:lower:]')" in
    x64)   echo "amd64" ;;
    x86)   echo "386" ;;
    arm64) echo "arm64" ;;
    arm)   echo "arm" ;;
    *)     error "Unsupported architecture: ${runner_arch}" ;;
  esac
}

# Verify checksum
verify_checksum() {
  local file="$1"
  local checksums_file="$2"
  local artifact_name="$3"  # The original artifact name in SHA256SUMS
  
  info "Verifying checksum..."
  
  if [[ ! -f "$checksums_file" ]]; then
    warn "Checksums file not found, skipping verification"
    return 0
  fi
  
  local expected
  expected=$(grep -F "${artifact_name}" "$checksums_file" | head -1 | awk '{print $1}')
  
  if [[ -z "$expected" ]]; then
    warn "Could not find checksum for ${artifact_name} in checksums file, skipping verification"
    return 0
  fi
  
  local actual
  local file_dir
  local file_name
  file_dir=$(dirname "$file")
  file_name=$(basename "$file")
  
  # Change to file directory to avoid path issues on Windows
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(cd "$file_dir" && sha256sum "$file_name" 2>/dev/null | grep -oE '^[a-f0-9]+') || true
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(cd "$file_dir" && shasum -a 256 "$file_name" 2>/dev/null | grep -oE '^[a-f0-9]+') || true
  else
    warn "Neither sha256sum nor shasum available, skipping verification"
    return 0
  fi
  
  if [[ -z "$actual" ]]; then
    error "Failed to calculate checksum for ${artifact_name}"
  fi
  
  if [[ "$expected" != "$actual" ]]; then
    error "Checksum verification failed!\nExpected: ${expected}\nActual:   ${actual}"
  fi
  
  info "Checksum verified successfully"
}

# Main function
main() {
  info "Setting up Ghoten..."
  
  # Version is already resolved and passed via INPUT_GHOTEN_VERSION (without 'v' prefix)
  local version_clean="${INPUT_GHOTEN_VERSION:-}"
  if [[ -z "$version_clean" ]]; then
    error "INPUT_GHOTEN_VERSION is required"
  fi
  
  local version_tag="v${version_clean}"
  
  info "Version: ${version_tag}"
  
  # Detect platform
  local os arch
  os=$(map_os)
  arch=$(map_arch)
  info "Platform: ${os}/${arch}"
  
  # Build artifact name
  local binary_suffix=""
  if [[ "$os" = "windows" ]]; then
    binary_suffix=".exe"
  fi
  
  # Determine archive format (prefer .zip for cross-platform compatibility)
  local archive_ext=".zip"
  local artifact_name="ghoten_${version_clean}_${os}_${arch}${archive_ext}"
  local checksums_file="ghoten_${version_clean}_SHA256SUMS"
  
  # Create tool cache directory
  local tool_dir="${RUNNER_TOOL_CACHE:-/tmp}/ghoten/${version_clean}/${arch}"
  mkdir -p "$tool_dir"
  
  # Binary path
  local binary_path="${tool_dir}/${BINARY_NAME}${binary_suffix}"
  
  # Check if binary was restored from cache
  if [[ "${CACHE_HIT:-}" = "true" ]] && [[ -x "$binary_path" ]]; then
    info "Using cached binary at ${binary_path}"
    # GitHub Actions cache already validates integrity, no need to re-verify checksum
  fi
  
  if [[ "${CACHE_HIT:-}" != "true" ]]; then
    # Build download URLs
    local base_url="https://github.com/${GITHUB_REPO}/releases/download/${version_tag}"
    local binary_url="${base_url}/${artifact_name}"
    local checksums_url="${base_url}/${checksums_file}"
    
    info "Downloading ${artifact_name}..."
  
    local curl_opts=(-fsSL)
    if [[ -n "${GITHUB_TOKEN:-}" ]]; then
      curl_opts+=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
    fi
    
    # Download archive
    local archive_path="${tool_dir}/${artifact_name}"
    if ! curl "${curl_opts[@]}" "$binary_url" -o "$archive_path"; then
      error "Failed to download binary from ${binary_url}"
    fi
    
    # Download checksums
    local checksums_path="${tool_dir}/${checksums_file}"
    if curl "${curl_opts[@]}" "$checksums_url" -o "$checksums_path" 2>/dev/null; then
      # Verify checksum of the archive
      verify_checksum "$archive_path" "$checksums_path" "$artifact_name"
    else
      warn "Could not download ${checksums_file}, skipping checksum verification"
    fi
    
    # Extract archive
    info "Extracting archive..."
    if command -v unzip >/dev/null 2>&1; then
      if ! unzip -q -o "$archive_path" -d "$tool_dir"; then
        error "Failed to extract archive"
      fi
    else
      error "unzip command not found, cannot extract archive"
    fi
    
    # Clean up archive
    rm -f "$archive_path"
    
    # Verify binary was extracted
    if [[ ! -f "$binary_path" ]]; then
      error "Binary not found after extraction: ${binary_path}"
    fi
    
    # Make executable
    chmod +x "$binary_path"
  fi
  
  # Verify installation
  info "Verifying installation..."
  local installed_version
  # Use head -1 with || true to ignore SIGPIPE (exit code 141)
  installed_version=$("${binary_path}" version 2>/dev/null | head -1 || true)
  if [[ -z "$installed_version" ]]; then
    error "Installation verification failed - binary does not execute"
  fi
  info "Installed: ${installed_version}"
  
  # Add to PATH
  info "Adding ${tool_dir} to PATH"
  echo "${tool_dir}" >> "${GITHUB_PATH}"
  
  # Create alias if requested
  if [[ -n "${INPUT_ALIAS:-}" ]]; then
    # Validate alias to prevent command injection and path traversal
    if ! [[ "${INPUT_ALIAS}" =~ ^[A-Za-z0-9_-]+$ ]]; then
      error "Invalid alias '${INPUT_ALIAS}'. Only letters, numbers, hyphens, and underscores are allowed."
    fi
    
    info "Creating alias: ${INPUT_ALIAS} -> ghoten"
    local alias_dir="${tool_dir}"
    
    # Create alias script
    if [[ "$os" = "windows" ]]; then
      # Windows batch file
      local alias_file="${alias_dir}/${INPUT_ALIAS}.bat"
      cat > "${alias_file}" << 'EOF'
@echo off
ghoten %*
exit /b %errorlevel%
EOF
      chmod +x "${alias_file}"
    else
      # Unix shell script
      local alias_file="${alias_dir}/${INPUT_ALIAS}"
      cat > "${alias_file}" << 'EOF'
#!/bin/bash
ghoten "$@"
EOF
      chmod +x "${alias_file}"
    fi
    
    info "Alias created at: ${alias_file}"
  fi
  
  # Set output
  echo "version=${version_clean}" >> "${GITHUB_OUTPUT}"
  
  info "Ghoten ${version_tag} has been installed successfully!"
}

main "$@"
