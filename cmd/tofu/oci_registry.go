// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"log"

	libregistryLogger "github.com/opentofu/libregistry/logger"
	"github.com/opentofu/libregistry/registryprotocols/ociclient"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
)

func newOCIDistributionClient(_ *cliconfig.OCIRegistries) (ociclient.OCIClient, error) {
	ociLogger := libregistryLogger.NewGoLogLogger(log.Default())
	// TODO: Also transform the information in the given config to
	// a suitable credentials configuration for the client, once
	// there are options for that. For now we just ignore the
	// configuration completely.
	return ociclient.New(ociclient.WithLogger(ociLogger))
}
