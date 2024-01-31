# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

zypper --gpg-auto-import-keys refresh opentofu
zypper --gpg-auto-import-keys refresh opentofu-source
zypper install -y tofu