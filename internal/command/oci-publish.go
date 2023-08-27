// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
)

// OCIPublishCommand is a Command implementation that takes a Terraform
// configuration and outputs the dependency tree in graphical form.
type OCIPublishCommand struct {
	Meta
}

func (c *OCIPublishCommand) Run(args []string) int {
	var verbose bool

	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("oci-publish")
	cmdFlags.BoolVar(&verbose, "verbose", false, "verbose")
	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	ref, err := registry.ParseReference(args[0])
	if err != nil {
		panic(err)
	}

	repository := fmt.Sprintf("%s/%s", ref.Registry, ref.Repository)
	tag := ref.Reference

	fs, err := file.NewWithFallbackLimit("./", 1024*1024*128)
	if err != nil {
		panic(err)
	}
	defer fs.Close()

	dir, err := os.ReadDir(".")
	if err != nil {
		panic(err)
	}

	var packageDescriptors []ocispec.Descriptor
	for _, entry := range dir {
		baseName := strings.TrimSuffix(entry.Name(), ".zip")
		parts := strings.Split(baseName, "_")
		os, arch := parts[2], parts[3]

		fileDesc, err := fs.Add(c.CommandContext(), entry.Name(), "example/file", entry.Name())
		if err != nil {
			panic(err)
		}
		c.Ui.Output(fmt.Sprintf("Adding %s: %s", entry.Name(), fileDesc.Digest))

		artifactType := "example/platform-package"
		packageDesc, err := oras.Pack(c.CommandContext(), fs, artifactType, []ocispec.Descriptor{fileDesc}, oras.PackOptions{
			PackImageManifest: true,
		})
		if err != nil {
			panic(err)
		}
		packageDesc.Platform = &ocispec.Platform{
			Architecture: arch,
			OS:           os,
		}
		c.Ui.Output(fmt.Sprintf("Prepared package for %s_%s: %s", os, arch, packageDesc.Digest))

		packageDescriptors = append(packageDescriptors, packageDesc)
	}

	// Create multi-platform artifact.
	listDesc := ocispec.Index{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType:    "application/vnd.docker.distribution.manifest.list.v2+json",
		ArtifactType: "example/multiplatform-package",
		Manifests:    packageDescriptors,
	}
	data, err := json.Marshal(listDesc)
	if err != nil {
		panic(err)
	}
	pushedDesc, err := oras.PushBytes(c.CommandContext(), fs, "application/vnd.docker.distribution.manifest.list.v2+json", data)
	if err != nil {
		panic(err)
	}
	c.Ui.Output(fmt.Sprintf("Prepared multi-platform package: %s", pushedDesc.Digest))

	if err = fs.Tag(c.CommandContext(), pushedDesc, tag); err != nil {
		panic(err)
	}

	repo, err := remote.NewRepository(repository)
	if err != nil {
		panic(err)
	}
	repo.PlainHTTP = true

	c.Ui.Output("")
	c.Ui.Output("Pushing to remote...")

	_, err = oras.Copy(c.CommandContext(), fs, tag, repo, tag, oras.CopyOptions{
		CopyGraphOptions: oras.CopyGraphOptions{
			OnCopySkipped: func(ctx context.Context, desc ocispec.Descriptor) error {
				c.Ui.Output(fmt.Sprintf("%s already exists in repository, skipping", desc.Digest))
				return nil
			},
			PostCopy: func(ctx context.Context, desc ocispec.Descriptor) error {
				c.Ui.Output(fmt.Sprintf("%s pushed", desc.Digest))
				return nil
			},
		},
	})
	if err != nil {
		panic(err)
	}

	c.Ui.Output("")
	c.Ui.Output(fmt.Sprintf("Successfully pushed %s", args[0]))

	c.Ui.Output("")

	return 0
}

func (c *OCIPublishCommand) Help() string {
	helpText := ``
	return strings.TrimSpace(helpText)
}

func (c *OCIPublishCommand) Synopsis() string {
	return ""
}
