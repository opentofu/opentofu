package oci

import (
	"fmt"

	"github.com/opencontainers/image-spec/specs-go"
	spec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/ociclient"
	"oras.land/oras-go/v2/content"
)

const (
	TOFU_LAYER_TYPE = "application/vnd.tofu.module.v1.tar+gzip"
	ARTIFACT_TYPE   = "application/vnd.tofu.module.manifest.v1+json"
)

func PushPackagedModule(ref string, src string) error {
	client := ociclient.New()
	err := client.GetCredentials(ref)
	if err != nil {
		return err
	}

	data, err := compressDir(src)
	if err != nil {
		return err
	}
	dd, data_opts := createBlobOptions(TOFU_LAYER_TYPE, ref, data)
	if dd.Size <= 0 {
		return fmt.Errorf("invalid digest")
	}

	// empty config, we can populate metadata if needed in the future.
	cd, config_opts := createBlobOptions(spec.MediaTypeEmptyJSON, ref, []byte("{}"))

	manifest := spec.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType:    spec.MediaTypeImageManifest,
		ArtifactType: ARTIFACT_TYPE,
		Config:       cd,
		Layers:       []spec.Descriptor{dd},
	}

	manifest_opts := createManifestOptions(manifest, ref)

	err = client.PushBlob(config_opts)
	if err != nil {
		return err
	}

	err = client.PushBlob(data_opts)
	if err != nil {
		return err
	}

	err = client.PushManifest(manifest_opts)
	if err != nil {
		return err
	}

	return nil
}

func createBlobOptions(mediaType string, ref string, blob []byte) (digest spec.Descriptor, opts ociclient.PushBlobOptions) {
	digest = content.NewDescriptorFromBytes(mediaType, blob)
	opts = ociclient.PushBlobOptions{
		Ref:      ref,
		Blob:     blob,
		Insecure: false,
	}

	return digest, opts
}

func createManifestOptions(manifest spec.Manifest, ref string) ociclient.PushManifestOptions {
	return ociclient.PushManifestOptions{
		Manifest: manifest,
		Ref:      ref,
		Insecure: false,
	}
}
