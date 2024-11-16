package getmodules

import (
	"fmt"
	"net/url"

	getter "github.com/hashicorp/go-getter"
)

type OciGetter struct{}

func (g *OciGetter) Get(urlStr string, u *url.URL) error {
	fmt.Println("--- OCI DEBUG:" + urlStr)
	return nil
}

func (g *OciGetter) GetFile(urlStr string, u *url.URL) error {
	// Implementation details...
	return nil
}

func (g *OciGetter) ClientMode(u *url.URL) (getter.ClientMode, error) {
	return getter.ClientModeDir, nil
}

func (g *OciGetter) SetClient(c *getter.Client) {}
