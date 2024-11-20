// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/libregistry/registryprotocols/ociclient"
	"io"
	"os"
	"path"
)

type ociImagePackageLocation struct {
	metadata ociclient.OCIImageMetadata
	client   ociclient.OCIClient
}

func (o ociImagePackageLocation) InstallProviderPackage(ctx context.Context, _ PackageMeta, targetDir string, allowedHashes []Hash) (*PackageAuthenticationResult, error) {
	diags := hcl.Diagnostics{}
	image, warnings, err := o.client.PullImageWithMetadata(ctx, o.metadata)
	for _, warning := range warnings {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  warning,
		})
	}
	if err != nil {
		// TODO the original error is lost here
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  err.Error(),
		})
		return nil, diags
	}
	for {
		next, err := image.Next()
		if err != nil {
			return nil, err
		}
		if !next {
			break
		}
		fn := image.Filename()
		info := image.FileInfo()
		targetFile := path.Join(targetDir, fn)
		// TODO do we need to validate path names here?
		if info.IsDir() {
			if err := os.MkdirAll(targetFile, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory %s (%w)", targetFile, err)
			}
		} else {
			targetFileDir := path.Dir(targetFile)
			if err := os.MkdirAll(targetFileDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory %s (%w)", targetFileDir, err)
			}
			fh, err := os.Create(targetFile)
			if err != nil {
				return nil, fmt.Errorf("failed to open %s for writing (%w)", targetFile, err)
			}
			if _, err := io.Copy(fh, image); err != nil {
				_ = fh.Close()
				return nil, fmt.Errorf("failed to write %s (%w)", targetFile, err)
			}
			if err := fh.Close(); err != nil {
				return nil, fmt.Errorf("failed to close %s (%w)", targetFile, err)
			}
			if err := os.Chtimes(targetFile, info.ModTime(), info.ModTime()); err != nil {
				return nil, fmt.Errorf("failed to change file modified time of %s (%w)", targetFile, err)
			}
		}
		if err := os.Chmod(targetFile, info.Mode()); err != nil {
			return nil, fmt.Errorf("failed to change mode of %s to %d (%w)", targetFile, info.Mode(), err)
		}
	}
	// TODO how do you add package authentication here?
	return nil, nil
}

func (o ociImagePackageLocation) String() string {
	return o.metadata.Addr.String()
}
