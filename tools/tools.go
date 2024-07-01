// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build tools
// +build tools

package tools

import (
	_ "github.com/mitchellh/gox"
	_ "github.com/nishanths/exhaustive"
	_ "go.uber.org/mock/mockgen"
	_ "golang.org/x/tools/cmd/stringer"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "honnef.co/go/tools/cmd/staticcheck"
)
