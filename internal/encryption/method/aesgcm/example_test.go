package aesgcm_test

import (
	"encoding/json"
	"fmt"

	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/errorhandling"
)

func Example() {
	descriptor := aesgcm.New()

	// Get the config struct. You can fill it manually by type-asserting it to aesgcm.Config, but you could also use
	// it as JSON.
	config := descriptor.ConfigStruct()

	errorhandling.Must(json.Unmarshal(
		// Set up a randomly generated 32-byte key. In JSON, you can base64-encode the value.
		[]byte(`{
    "key": "Y29veTRhaXZ1NWFpeW9vMWlhMG9vR29vVGFlM1BhaTQ="
}`), &config))

	method := errorhandling.Must2(config.Build())

	// Encrypt some data:
	encrypted := errorhandling.Must2(method.Encrypt([]byte("Hello world!")))

	// Now decrypt it:
	decrypted := errorhandling.Must2(method.Decrypt(encrypted))

	fmt.Printf("%s", decrypted)
	// Output: Hello world!
}
