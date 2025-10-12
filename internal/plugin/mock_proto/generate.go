// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate go tool go.uber.org/mock/mockgen -destination mock.go github.com/opentofu/opentofu/internal/tfplugin5 ProviderClient,ProvisionerClient,Provisioner_ProvisionResourceClient,Provisioner_ProvisionResourceServer

package mock_tfplugin5
