package encryption

import (
	"fmt"
	"testing"
)

func Test(t *testing.T) {
	reg := NewRegistry()

	cfgA, diags := LoadConfigFromString("Test Source A", `
backend {
	enforced = true
}
`)
	if diags.HasErrors() {
		t.Error(diags.Error())
	}

	cfgB, diags := LoadConfigFromString("Test Source B", `
key_provider "passphrase" "basic" {
	passphrase = "fuzzybunnyslippers"
}
method "aes_cipher" "example" {
	cipher = key_provider.passphrase.basic
}
backend {
	method = "method.aes_cipher.example" # See EvalContext comment in encryption.go
}
`)
	if diags.HasErrors() {
		t.Error(diags.Error())
	}

	cfgA.ApplyOverrides(cfgB)

	enc, diags := New(reg, cfgA)

	for _, d := range diags {
		println(d.Error())
	}

	if diags.HasErrors() {
		t.Error(diags.Error())
	}

	fmt.Printf("%#v\n", enc)
}
