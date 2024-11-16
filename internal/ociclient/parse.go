package ociclient

import (
	"fmt"
	"regexp"

	"github.com/opencontainers/go-digest"
	spec "github.com/opencontainers/image-spec/specs-go/v1"
)

type Reference struct {
	Host      string
	Name      string
	Namespace string
	Version   string
}

func ParseRef(ref string) (Reference, error) {
	pattern := `^(?:(?P<host>[a-zA-Z0-9.-]+(?::[0-9]+)?)\/)?(?:(?P<namespace>[a-zA-Z0-9-._]+)\/)?(?P<name>[a-zA-Z0-9-._]+)(?::(?P<tag>[a-zA-Z0-9-._]+))?$`
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(ref)
	if matches == nil {
		return Reference{}, fmt.Errorf("invalid Docker image URL")
	}

	groupNames := re.SubexpNames()
	result := make(map[string]string)
	for i, name := range groupNames {
		if i != 0 && name != "" {
			result[name] = matches[i]
		}
	}
	image := Reference{
		Host:      result["host"],
		Namespace: result["namespace"],
		Name:      result["name"],
		Version:   result["tag"],
	}

	if image.Version == "" {
		image.Version = "latest"
	}

	return image, nil
}

func GetBlobDescriptor(mediaType string, data []byte) spec.Descriptor {
	return spec.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
	}
}
