package encryption

import "fmt"

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

func MethodAddr(method string, name string) string {
	return fmt.Sprintf("method.%s.%s", method, name)
}
