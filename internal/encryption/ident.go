package encryption

import (
	"fmt"
)

func KeyProviderAddr(provider string, name string) string {
	return fmt.Sprintf("key_provider.%s.%s", provider, name)
}
func MethodAddr(method string, name string) string {
	return fmt.Sprintf("method.%s.%s", method, name)
}
