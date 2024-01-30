package encryption

import (
	"fmt"
	"strings"
)

const (
	ConfigKeyProvider = "key_provider"
	ConfigKeyMethod   = "method"

	ConfigKeyBackend   = "backend"
	ConfigKeyStateFile = "statefile"
	ConfigKeyPlanFile  = "planfile"
	ConfigKeyRemote    = "remote_data_source"
)

func KeyProviderAddr(provider string, name string) string {
	return fmt.Sprintf("key_provider.%s.%s", provider, name)
}

func KeyProviderType(ident string) string {
	// TODO defense
	return strings.Split(ident, ".")[1]
}

func KeyProviderName(ident string) string {
	// TODO defense
	return strings.Split(ident, ".")[2]
}

func MethodAddr(method string, name string) string {
	return fmt.Sprintf("method.%s.%s", method, name)
}
func MethodType(ident string) string {
	// TODO defense
	return strings.Split(ident, ".")[1]
}
