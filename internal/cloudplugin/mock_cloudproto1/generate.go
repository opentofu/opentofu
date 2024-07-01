// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate go run go.uber.org/mock/mockgen -destination mock.go github.com/opentofu/opentofu/internal/cloudplugin/cloudproto1 CommandServiceClient,CommandService_ExecuteClient

package mock_cloudproto1
