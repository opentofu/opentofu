package getmodules

import (
	"net/url"

	getter "github.com/hashicorp/go-getter"
	"github.com/opentofu/opentofu/internal/oci"
)

type OciGetter struct {
	getter.Getter
}

func (g *OciGetter) Get(dst string, u *url.URL) error {
	ref := u.Host + u.Path
	return oci.PullModule(ref, dst)
}

func (g *OciGetter) GetFile(dst string, u *url.URL) error {
	// Not needed for OCI
	return nil
}

func (g *OciGetter) ClientMode(u *url.URL) (getter.ClientMode, error) {
	return getter.ClientModeDir, nil
}

func (g *OciGetter) SetClient(c *getter.Client) {}
