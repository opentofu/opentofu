// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"
	"testing"

	"github.com/apparentlymart/go-versions/versions"
	svchost "github.com/hashicorp/terraform-svchost"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestOCIMirrorSource(t *testing.T) {
	// This is just a stub test for now, since the source itself is just a stub.
	// We'll replace this with a more complete test once we have a more complete implementation!

	source := NewOCIMirrorSource(func(providerAddr addrs.Provider) (string, tfdiags.Diagnostics) {
		return fmt.Sprintf("example.net/%s/%s/%s", providerAddr.Hostname, providerAddr.Namespace, providerAddr.Type), nil
	})

	ctx := context.Background()
	providerAddr := addrs.Provider{
		Hostname:  svchost.Hostname("example.com"),
		Namespace: "foo",
		Type:      "bar",
	}
	platform := Platform{
		OS:   "msdos",
		Arch: "286",
	}
	_, _, err := source.AvailableVersions(ctx, providerAddr)
	if got, want := err.Error(), `would have listed available provider versions from example.net/example.com/foo/bar, but this provider installation method is not yet implemented`; got != want {
		t.Errorf("wrong error from AvailableVersions\ngot:  %s\nwant: %s", got, want)
	}
	_, err = source.PackageMeta(ctx, providerAddr, versions.MustParseVersion("1.0.0"), platform)
	if got, want := err.Error(), `would have fetched metadata from example.net/example.com/foo/bar:v1.0.0 for msdos_286, but this provider installation method is not yet implemented`; got != want {
		t.Errorf("wrong error from PackageMeta\ngot:  %s\nwant: %s", got, want)
	}
}
