package oci

import (
	"fmt"

	"github.com/opencontainers/image-spec/specs-go"
	spec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opentofu/opentofu/internal/ociclient"
)

func PushPackagedModule(ref string, src string, insecure bool) error {
	client := ociclient.New()
	err := client.GetCredentials(ref)
	if err != nil {
		return err
	}

	data, err := compressDir(src)
	if err != nil {
		return err
	}
	dd, data_opts := createBlobPushOptions(ociclient.TOFU_LAYER_TYPE, ref, data, insecure)
	if dd.Size <= 0 {
		return fmt.Errorf("invalid digest")
	}

	// empty config, we can populate metadata if needed in the future.
	cd, config_opts := createBlobPushOptions(spec.MediaTypeEmptyJSON, ref, []byte("{}"), insecure)

	manifest := spec.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType:    spec.MediaTypeImageManifest,
		ArtifactType: ociclient.ARTIFACT_TYPE,
		Config:       cd,
		Layers:       []spec.Descriptor{dd},
	}

	manifest_opts := createManifestPushOptions(manifest, ref, insecure)

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

func createBlobPushOptions(mediaType string, ref string, blob []byte, insecure bool) (digest spec.Descriptor, opts ociclient.PushBlobOptions) {
	digest = ociclient.GetBlobDescriptor(mediaType, blob)
	opts = ociclient.PushBlobOptions{
		Ref:      ref,
		Blob:     blob,
		Insecure: insecure,
	}

	return digest, opts
}

func createManifestPushOptions(manifest spec.Manifest, ref string, insecure bool) ociclient.PushManifestOptions {
	return ociclient.PushManifestOptions{
		Manifest: manifest,
		Ref:      ref,
		Insecure: insecure,
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
