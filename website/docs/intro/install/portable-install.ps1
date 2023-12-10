$TOFU_VERSION = "1.6.0-alpha2"

$TARGET = Join-Path $env:LOCALAPPDATA "OpenTofu"
if (-not (Test-Path $TARGET)) {
    Write-Host "Creating target directory: $TARGET"
    New-Item -ItemType Directory -Path $TARGET | Out-Null
}

# Change to the target directory
Push-Location $TARGET

# Download and extract OpenTofu
try {
    $downloadUrl = "https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_windows_amd64.zip"
    Write-Host "Downloading OpenTofu..."
    Invoke-WebRequest -Uri $downloadUrl -OutFile "tofu_${TOFU_VERSION}_windows_amd64.zip"
    Write-Host "Extracting OpenTofu..."
    Expand-Archive "tofu_${TOFU_VERSION}_windows_amd64.zip" -DestinationPath $TARGET -Force
    Write-Host "Cleaning up..."
    Remove-Item "tofu_${TOFU_VERSION}_windows_amd64.zip"
} catch {
    Write-Error "An error occurred while downloading OpenTofu: $_"
    exit 1
} finally {
    Pop-Location # Change back to the original directory
}

$TOFU_PATH = Join-Path $TARGET "tofu.exe"

Write-Host "OpenTofu is now available at ${TOFU_PATH}." 
Write-Warning "Please add ${TOFU_PATH} to your PATH environment variable for easier access." 
Write-Warning "On Windows, this may require a logoff/logon or reboot."