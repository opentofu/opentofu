package ociclient

import (
	"fmt"
	"regexp"
)

type OciReference struct {
	Host      string
	Name      string
	Namespace string
	Version   string
}

func ParseRef(tag string) (OciReference, error) {
	pattern := `^(?:(?P<host>[a-zA-Z0-9.-]+(?::[0-9]+)?)\/)?(?:(?P<namespace>[a-zA-Z0-9-._]+)\/)?(?P<name>[a-zA-Z0-9-._]+)(?::(?P<tag>[a-zA-Z0-9-._]+))?$`
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(tag)
	if matches == nil {
		return OciReference{}, fmt.Errorf("invalid Docker image URL")
	}

	groupNames := re.SubexpNames()
	result := make(map[string]string)
	for i, name := range groupNames {
		if i != 0 && name != "" {
			result[name] = matches[i]
		}
	}
	image := OciReference{
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
