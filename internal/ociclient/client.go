package ociclient

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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
			return nil, fmt.Errorf("unauthorized, please use docker login to authenticate")
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

	endpoint := getManifestEndpont(ref, false)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", spec.MediaTypeImageManifest)
	if c.Credentials != nil {
		req.Header.Add("Authorization", c.Credentials.encoded)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("unauthorized, please use nori login to authenticate")
		}

		return nil, fmt.Errorf("cannot to pull manifest: %s", resp.Status)
	}

	manifestBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	manifest := &spec.Manifest{}
	err = json.Unmarshal(manifestBytes, manifest)
	if err != nil {
		return nil, err
	}

	return manifest, nil
}

func (c *Client) PushBlob(opts PushBlobOptions) error {
	ref, err := ParseRef(opts.Ref)
	if err != nil {
		return err
	}
	protocol := getHTTPProtocol(opts.Insecure)
	var endpoint string
	if ref.Namespace != "" {
		endpoint = fmt.Sprintf("%s://%s/v2/%s/%s/blobs/uploads/", protocol, ref.Host, ref.Namespace, ref.Name)
	} else {
		endpoint = fmt.Sprintf("%s://%s/v2/%s/blobs/uploads/", protocol, ref.Host, ref.Name)
	}

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %s", err.Error())
	}

	if c.Credentials != nil {
		req.Header.Add("Authorization", c.Credentials.encoded)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %s", err.Error())
	}

	if resp.StatusCode != 202 {
		return fmt.Errorf("failed to push blob: %s", resp.Status)
	}

	location := resp.Header.Get("Location")
	req, err = http.NewRequest("PUT", location, bytes.NewReader(opts.Blob))
	if err != nil {
		return fmt.Errorf("error uploading blob: %s", err.Error())
	}

	req.Header.Add("Content-Type", "application/octet-stream")
	req.Header.Add("Content-Length", fmt.Sprintf("%d", len(opts.Blob)))
	query := req.URL.Query()
	query.Add("digest", "")
	req.URL.RawQuery = query.Encode()

	if c.Credentials != nil {
		req.Header.Add("Authorization", c.Credentials.encoded)
	}

	resp, err = client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 201 {
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("unauthorized, please use docker login to authenticate")
		}
		return fmt.Errorf("failed to push blob: %s", resp.Status)
	}

	return nil
}

func (c *Client) PushManifest(opts PushManifestOptions) error {
	ref, err := ParseRef(opts.Ref)
	if err != nil {
		return err
	}
	endpoint := getManifestEndpont(ref, opts.Insecure)
	req, err := http.NewRequest("HEAD", endpoint, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %s", err.Error())
	}

	if c.Credentials != nil {
		req.Header.Add("Authorization", c.Credentials.encoded)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %s", err.Error())
	}

	jsonBytes, err := json.Marshal(opts.Manifest)
	if err != nil {
		return fmt.Errorf("error marshalling manifest: %s", err.Error())
	}

	if resp.StatusCode != 200 {
		uploadReq, err := http.NewRequest("PUT", endpoint, bytes.NewReader(jsonBytes))
		if err != nil {
			return fmt.Errorf("error creating request: %s", err.Error())
		}

		uploadReq.Header.Add("Content-Type", spec.MediaTypeImageManifest)
		uploadReq.Header.Add("Content-Length", fmt.Sprintf("%d", len(jsonBytes)))

		if c.Credentials != nil {
			uploadReq.Header.Add("Authorization", c.Credentials.encoded)
		}

		resp, err = client.Do(uploadReq)
		if err != nil {
			return fmt.Errorf("error sending request: %s", err.Error())
		}

		if resp.StatusCode != 201 {
			if resp.StatusCode == http.StatusUnauthorized {
				return fmt.Errorf("unauthorized, please use nori login to authenticate")
			}
			return fmt.Errorf("failed to push manifest: %s", resp.Status)
		}
	}

	return nil
}

func getBlobEndpont(ref OciReference, digest string) string {
	if ref.Namespace != "" {
		return fmt.Sprintf("https://%s/v2/%s/%s/blobs/%s", ref.Host, ref.Namespace, ref.Name, digest)
	} else {
		return fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.Host, ref.Name, digest)

	}
}

func getManifestEndpont(ref OciReference, insecure bool) string {
	protocol := getHTTPProtocol(insecure)
	if ref.Namespace != "" {
		return fmt.Sprintf("%s://%s/v2/%s/%s/manifests/%s", protocol, ref.Host, ref.Namespace, ref.Name, ref.Version)
	} else {
		return fmt.Sprintf("%s://%s/v2/%s/manifests/%s", protocol, ref.Host, ref.Name, ref.Version)
	}
}

func getHTTPProtocol(insecure bool) string {
	if insecure {
		return "http"
	}

	return "https"
}
