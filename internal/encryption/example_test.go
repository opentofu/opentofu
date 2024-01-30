package encryption

import "testing"

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
backend {
	method = "method.foo.bar" # See EvalContext comment in encryption.go
}
`)
	if diags.HasErrors() {
		t.Error(diags.Error())
	}

	cfgA.ApplyOverrides(cfgB)

	enc, diags := New(reg, cfgA)

	println(enc)
}
