package aesgcm_test

import (
	"encoding/json"
	"fmt"

	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
)

func Example() {
	descriptor := aesgcm.New()

	// Get the config struct. You can fill it manually by type-asserting it to aesgcm.Config, but you could also use
	// it as JSON.
	config := descriptor.ConfigStruct()

	if err := json.Unmarshal(
		// Set up a randomly generated 32-byte key. In JSON, you can base64-encode the value.
		[]byte(`{
    "key": "Y29veTRhaXZ1NWFpeW9vMWlhMG9vR29vVGFlM1BhaTQ="
}`), &config); err != nil {
		panic(err)
	}

	method, err := config.Build()
	if err != nil {
		panic(err)
	}

	// Encrypt some data:
	encrypted, err := method.Encrypt([]byte("Hello world!"))
	if err != nil {
		panic(err)
	}

	// Now decrypt it:
	decrypted, err := method.Decrypt(encrypted)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s", decrypted)
	// Output: Hello world!
}
