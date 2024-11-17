package oci

import (
	"fmt"

	"github.com/opencontainers/image-spec/specs-go"
	spec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/ociclient"
	"oras.land/oras-go/v2/content"
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
	dd, data_opts := createBlobPushOptions(ociclient.TOFU_LAYER_TYPE, ref, data)
	if dd.Size <= 0 {
		return fmt.Errorf("invalid digest")
	}

	// empty config, we can populate metadata if needed in the future.
	cd, config_opts := createBlobPushOptions(spec.MediaTypeEmptyJSON, ref, []byte("{}"))

	manifest := spec.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType:    spec.MediaTypeImageManifest,
		ArtifactType: ociclient.ARTIFACT_TYPE,
		Config:       cd,
		Layers:       []spec.Descriptor{dd},
	}

	manifest_opts := createManifestPushOptions(manifest, ref)

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

func createBlobPushOptions(mediaType string, ref string, blob []byte) (digest spec.Descriptor, opts ociclient.PushBlobOptions) {
	digest = content.NewDescriptorFromBytes(mediaType, blob)
	opts = ociclient.PushBlobOptions{
		Ref:      ref,
		Blob:     blob,
		Insecure: false,
	}

	return digest, opts
}

func createManifestPushOptions(manifest spec.Manifest, ref string) ociclient.PushManifestOptions {
	return ociclient.PushManifestOptions{
		Manifest: manifest,
		Ref:      ref,
		Insecure: false,
	}
}

func PullModule(ref string, dst string) error {
	client := ociclient.New()
	if err := client.GetCredentials(ref); err != nil {
		return err
	}

	data, err := client.PullManifestContent(ref)
	if err != nil {
		return err
	}

	return decompressToDir(data, dst)
}
