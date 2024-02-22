package aesgcm_test

import (
	"fmt"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/errorhandling"
)

func Example_config() {
	// First, get the descriptor to make sure we always have the default values.
	descriptor := aesgcm.New()

	// The ConfigStruct returns an interface, you must type-assert it to aescgm.Config in order to fill it manually.
	// In the real use-case this will be filled based on the HCL tags.
	//
	// Note: do not create the Config struct yourself as it will not have its default values pre-filled.
	config := descriptor.ConfigStruct().(aesgcm.Config)

	// Set up an encryption key:
	config.Key = []byte("AiphoogheuwohShal8Aefohy7ooLeeyu")

	// Now you can build a method:
	method := errorhandling.Must2(config.Build())

	// Encrypt something:
	encrypted := errorhandling.Must2(method.Encrypt([]byte("Hello world!")))

	// Decrypt it:
	decrypted := errorhandling.Must2(method.Decrypt(encrypted))

	fmt.Printf("%s", decrypted)
	// Output: Hello world!
}
