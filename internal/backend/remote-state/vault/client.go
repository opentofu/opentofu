package vault

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// RemoteClient is a remote client that stores data in Vault.
type RemoteClient struct {
	Client *vault.Client
	Name   string
	Path   string
	GZip   bool

	ctx       context.Context
	mountPath string
}

func (c *RemoteClient) Get() (*remote.Payload, error) {
	resp, err := c.Client.Secrets.KvV2Read(c.ctx, c.Path, vault.WithMountPath(c.mountPath))
	if err != nil {
		re := err.(*vault.ResponseError)
		if re.StatusCode == http.StatusNotFound && len(re.Errors) == 0 {
			// No existing state returns empty.
			return nil, nil
		}
		return nil, err
	}

	var payload []byte

	if resp.Data.Data["tfstate"] != nil {
		if gz, ok := resp.Data.Data["gzip"].(bool); ok && gz {
			payload, err = uncompressState(resp.Data.Data["tfstate"].(string))
			if err != nil {
				return nil, err
			}
		} else {
			payload = []byte(resp.Data.Data["tfstate"].(string))
		}
	} else {
		// 204 No Content
		return nil, nil
	}

	md5 := md5.Sum(payload)

	return &remote.Payload{
		Data: payload,
		MD5:  md5[:],
	}, nil
}

func (c *RemoteClient) Put(data []byte) error {
	var err error
	var writeData = map[string]any{"tfstate": string(data)}

	if c.GZip {
		if writeData["tfstate"], err = compressState(data); err != nil {
			return err
		}
		writeData["gzip"] = true
	}

	_, err = c.Client.Secrets.KvV2Write(c.ctx, c.Path, schema.KvV2WriteRequest{
		Data: writeData,
	}, vault.WithMountPath(c.mountPath))

	return err
}

func (c *RemoteClient) Delete() error {
	_, err := c.Client.Secrets.KvV2Delete(c.ctx, c.Path, vault.WithMountPath(c.mountPath))

	return err
}

func (c *RemoteClient) Lock(info *statemgr.LockInfo) (string, error) {
	info.Path = c.Path

	if info.ID == "" {
		lockID, err := uuid.GenerateUUID()
		if err != nil {
			return "", &statemgr.LockError{Info: info, Err: err}
		}

		info.ID = lockID
	}

	respData, err := GetMetadata(c.ctx, c.Client, c.mountPath, c.Path, false)
	if err != nil {
		return "", &statemgr.LockError{
			Err: fmt.Errorf("failed to retrieve lock info: %s", err),
		}
	}

	if respData["custom_metadata"] != nil {
		CustomMetadata := respData["custom_metadata"].(map[string]any)
		id, ok := CustomMetadata["LockID"].(string)
		if ok && id != "" {
			// info.ID = id
			return "", &statemgr.LockError{Info: info, Err: fmt.Errorf("workspace is already locked: %s", c.Name)}
		}
	}

	_, err = c.Client.Secrets.KvV2WriteMetadata(c.ctx, c.Path, schema.KvV2WriteMetadataRequest{
		CustomMetadata: map[string]any{
			"LockID": info.ID,
			"Info":   string(info.Marshal()),
		},
	}, vault.WithMountPath(c.mountPath))
	if err != nil {
		return "", &statemgr.LockError{Info: info, Err: err}
	}

	return info.ID, nil
}

func (c *RemoteClient) Unlock(id string) error {
	respData, err := GetMetadata(c.ctx, c.Client, c.mountPath, c.Path, false)
	if err != nil {
		return &statemgr.LockError{
			Err: fmt.Errorf("failed to retrieve lock info: %s", err),
		}
	}

	if respData["custom_metadata"] == nil {
		return &statemgr.LockError{Err: fmt.Errorf("lock id %q does not match existing lock", id)}
	}

	CustomMetadata := respData["custom_metadata"].(map[string]any)
	if lid, ok := CustomMetadata["LockID"].(string); !ok || lid != id {
		return &statemgr.LockError{Err: fmt.Errorf("lock id %q does not match existing lock", id)}
	}

	delete(CustomMetadata, "LockID")

	_, err = c.Client.Secrets.KvV2WriteMetadata(c.ctx, c.Path, schema.KvV2WriteMetadataRequest{
		CustomMetadata: CustomMetadata,
	}, vault.WithMountPath(c.mountPath))
	if err != nil {
		return &statemgr.LockError{
			Err: fmt.Errorf("failed to delete lock info from metadata: %s", err),
		}
	}

	return nil
}

func GetMetadata(ctx context.Context, c *vault.Client, mountPath, path string, list bool) (map[string]any, error) {
	var opt string
	if list {
		opt = "?list=true"
	}

	resp, err := c.Read(ctx, fmt.Sprintf("/v1/%s/metadata/%v%v", mountPath, path, opt))
	if err != nil {
		re := err.(*vault.ResponseError)
		if re.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("please check that your access token is valid. Return code %s", re)
		} else if re.StatusCode == http.StatusNotFound && len(re.Errors) == 0 {
			// No existing records returns empty.
			return nil, nil
		}
		return nil, err
	}

	return resp.Data, nil
}

func compressState(data []byte) (string, error) {
	s := bytes.NewBufferString("")
	b64 := base64.NewEncoder(base64.StdEncoding, s)
	gz := gzip.NewWriter(b64)
	if _, err := gz.Write(data); err != nil {
		return "", err
	}
	if err := gz.Flush(); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	return s.String(), nil
}

func uncompressState(data string) ([]byte, error) {
	b := new(bytes.Buffer)
	b64 := base64.NewDecoder(base64.StdEncoding, bytes.NewBufferString(data))
	gz, err := gzip.NewReader(b64)
	if err != nil {
		return nil, err
	}
	b.ReadFrom(gz)
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
