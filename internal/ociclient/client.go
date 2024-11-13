package ociclient

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	spec "github.com/opencontainers/image-spec/specs-go/v1"
)

type PullBlobOptions struct {
	Ref        string
	Descriptor spec.Descriptor
}

type PushBlobOptions struct {
	Blob     []byte
	Ref      string
	Insecure bool
}

type PullManifestOptions struct {
	Ref string
}

type PushManifestOptions struct {
	Manifest spec.Manifest
	Ref      string
	Insecure bool
}

type Client struct {
	Credentials *Credentials
}

type Credentials struct {
	Username string
	Password string
	encoded  string
}

func New() *Client {
	return &Client{}
}

func (c *Client) SetBasicAuth(username, password string) {
	userpass := fmt.Sprintf("%s:%s", username, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(userpass))
	authHeader := fmt.Sprintf("Basic %s", encoded)
	c.Credentials = &Credentials{
		Username: username,
		Password: password,
		encoded:  authHeader,
	}
}

func (c *Client) PullBlob(opts PullBlobOptions) ([]byte, error) {
	ref, err := ParseRef(opts.Ref)
	if err != nil {
		return nil, fmt.Errorf("error parsing ref: %w", err)
	}
	endpoint := getBlobEndpont(ref, opts.Descriptor.Digest.String())

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating oci request: %s", err.Error())
	}

	if c.Credentials != nil {
		req.Header.Add("Authorization", c.Credentials.encoded)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %s", err.Error())
	}

	if resp.StatusCode != 200 {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("unauthorized, please use nori login to authenticate")
		}
		return nil, fmt.Errorf("failed to pull blob: %s", resp.Status)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading blob: %s", err.Error())
	}

	return data, nil
}

func (c *Client) PullManifest(opts PullManifestOptions) (*spec.Manifest, error) {
	ref, err := ParseRef(opts.Ref)
	if err != nil {
		return nil, fmt.Errorf("error parsing ref: %w", err)
	}
	return nil, nil
}

func (c *Client) PushBlob(opts PushBlobOptions) error {
	return nil
}

func (c *Client) PushManifest(opts PushManifestOptions) error {
	return nil
}

func getBlobEndpont(ref OciReference, digest string) string {
	if ref.Namespace != "" {
		return fmt.Sprintf("https://%s/v2/%s/%s/blobs/%s", ref.Host, ref.Namespace, ref.Name, digest)
	} else {
		return fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.Host, ref.Name, digest)

	}
}

func getManifestEndpont(ref OciReference, digest string) string {
	if ref.Namespace != "" {
		return fmt.Sprintf("https://%s/%s/%s/v2/%s/%s/manifests/%s", ref.Host, ref.Namespace, ref.Name, ref.Version, ref.Name, ref.Version)
	} else {
		return fmt.Sprintf("https://%s/v2/%s/%s/manifests/%s", ref.Host, ref.Name, ref.Version, ref.Name, ref.Version)
	}
}
