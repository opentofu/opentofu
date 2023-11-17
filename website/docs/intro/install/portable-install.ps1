$TOFU_VERSION="1.6.0-alpha2"
$TARGET=Join-Path $env:LOCALAPPDATA OpenTofu
New-Item -ItemType Directory -Path $TARGET
Push-Location $TARGET
Invoke-WebRequest -Uri "https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_windows_amd64.zip" -OutFile "tofu_${TOFU_VERSION}_windows_amd64.zip"
Expand-Archive "tofu_${TOFU_VERSION}_windows_amd64.zip" -DestinationPath $TARGET
Remove-Item "tofu_${TOFU_VERSION}_windows_amd64.zip"
$TOFU_PATH=Join-Path $TARGET tofu.exe
Pop-Location
echo "OpenTofu is now available at ${TOFU_PATH}. Please add it to your path for easier access."