package encryption_test

import (
	"fmt"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider/static"
	"github.com/opentofu/opentofu/internal/encryption/method/aesgcm"
	"github.com/opentofu/opentofu/internal/encryption/registry/lockingencryptionregistry"
)

func ExampleEncryption() {
	reg := lockingencryptionregistry.New()
	if err := reg.RegisterKeyProvider(static.New()); err != nil {
		panic(err)
	}
	if err := reg.RegisterMethod(aesgcm.New()); err != nil {
		panic(err)
	}

	cfgA, diags := encryption.LoadConfigFromString("Test Source A", `
backend {
	enforced = true
}
`)
	if diags.HasErrors() {
		panic(diags.Error())
	}

	cfgB, diags := encryption.LoadConfigFromString("Test Source B", `
key_provider "static" "basic" {
	key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
}
method "aes_gcm" "example" {
	cipher = key_provider.static.basic
}
backend {
	method = method.aes_cipher.example
}
`)
	if diags.HasErrors() {
		panic(diags.Error())
	}

	cfg := encryption.MergeConfigs(cfgA, cfgB)

	enc, diags := encryption.New(reg, cfg)

	for _, d := range diags {
		println(d.Error())
	}

	if diags.HasErrors() {
		panic(diags.Error())
	}

	fmt.Printf("%#v\n", enc)
	// Output: test
}
