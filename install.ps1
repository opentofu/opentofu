# OpenTofu ORAS Fork Installer for Windows
# Usage: irm https://raw.githubusercontent.com/vmvarela/opentofu/develop/install.ps1 | iex
#
# Or with options:
#   $env:TOFU_ORAS_VERSION = "v1.12.0-oci"
#   $env:TOFU_ORAS_INSTALL_DIR = "$env:USERPROFILE\.local\bin"
#   irm https://raw.githubusercontent.com/vmvarela/opentofu/develop/install.ps1 | iex

$ErrorActionPreference = "Stop"

$GitHubRepo = "vmvarela/opentofu"
$BinaryName = if ($env:TOFU_ORAS_BINARY_NAME) { $env:TOFU_ORAS_BINARY_NAME } else { "tofu-oras" }
$InstallDir = if ($env:TOFU_ORAS_INSTALL_DIR) { $env:TOFU_ORAS_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\tofu-oras" }

function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] " -ForegroundColor Green -NoNewline
    Write-Host $Message
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[WARN] " -ForegroundColor Yellow -NoNewline
    Write-Host $Message
}

function Write-Err {
    param([string]$Message)
    Write-Host "[ERROR] " -ForegroundColor Red -NoNewline
    Write-Host $Message
    exit 1
}

function Get-Architecture {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        "X64" { return "amd64" }
        "X86" { return "386" }
        "Arm64" { return "arm64" }
        default { Write-Err "Unsupported architecture: $arch" }
    }
}

function Get-LatestVersion {
    try {
        $response = Invoke-RestMethod -Uri "https://api.github.com/repos/$GitHubRepo/releases/latest" -UseBasicParsing
        return $response.tag_name
    }
    catch {
        Write-Err "Failed to fetch latest version: $_"
    }
}

function Add-ToPath {
    param([string]$Directory)
    
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($currentPath -notlike "*$Directory*") {
        [Environment]::SetEnvironmentVariable("Path", "$currentPath;$Directory", "User")
        $env:Path = "$env:Path;$Directory"
        Write-Info "Added $Directory to PATH"
        return $true
    }
    return $false
}

# Main
Write-Host ""
Write-Info "OpenTofu ORAS Fork Installer for Windows"
Write-Host ""

# Detect architecture
$Arch = Get-Architecture
Write-Info "Detected architecture: windows/$Arch"

# Get version
$Version = if ($env:TOFU_ORAS_VERSION) { $env:TOFU_ORAS_VERSION } else { $null }
if (-not $Version) {
    Write-Info "Fetching latest version..."
    $Version = Get-LatestVersion
}
Write-Info "Version: $Version"

# Build download URL
$ArtifactName = "tofu_windows_${Arch}.exe"
$DownloadUrl = "https://github.com/$GitHubRepo/releases/download/$Version/$ArtifactName"
Write-Info "Downloading from: $DownloadUrl"

# Create install directory
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Write-Info "Created directory: $InstallDir"
}

# Download
$TempFile = Join-Path $env:TEMP $ArtifactName
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $TempFile -UseBasicParsing
}
catch {
    Write-Err "Download failed: $_"
}

# Install
$InstallPath = Join-Path $InstallDir "$BinaryName.exe"
Move-Item -Path $TempFile -Destination $InstallPath -Force
Write-Info "Installed to: $InstallPath"

# Add to PATH
$PathAdded = Add-ToPath -Directory $InstallDir

# Verify
Write-Host ""
Write-Info "✅ Installation complete!"
Write-Host ""
Write-Info "Binary installed: $InstallPath"

try {
    $versionOutput = & $InstallPath version 2>$null
    Write-Info "Version: $versionOutput"
}
catch {
    Write-Info "Version: $Version"
}

Write-Host ""
Write-Info "Usage:"
Write-Host "  $BinaryName init"
Write-Host "  $BinaryName plan"
Write-Host "  $BinaryName apply"
Write-Host ""

if ($PathAdded) {
    Write-Warn "Please restart your terminal for PATH changes to take effect."
    Write-Host ""
}

Write-Info "Documentation: https://github.com/$GitHubRepo/blob/develop/internal/backend/remote-state/oras/README.md"
