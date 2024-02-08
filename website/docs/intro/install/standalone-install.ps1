# Copyright (c) The OpenTofu Authors
# SPDX-License-Identifier: MPL-2.0
# Copyright (c) 2023 HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Download the installer script:
Invoke-WebRequest -outfile "install-opentofu.ps1" -uri "https://get.opentofu.org/install-opentofu.ps1"

# Please inspect the downloaded script at this point.

# Run the installer:
& .\install-opentofu.ps1 -installMethod standalone

# Remove the installer:
Remove-Item install-opentofu.ps1