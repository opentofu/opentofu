package keyprovider

import "github.com/zclconf/go-cty/cty"

// Output is the standardized structure a key provider must return when providing a key.
// It contains two keys because some key providers may prefer include random data (e.g. salt)
// in the generated keys.
// Additionally, the Metadata should contain a struct with `meta` tags that OpenTofu can serialize
// into the encrypted form. OpenTofu will inject the `meta` tagged items back into the
// key provider configuration when an item needs to be decrypted.
type Output struct {
	EncryptionKey []byte `hcl:"encryption_key" cty:"encryption_key" json:"encryption_key" yaml:"encryption_key"`
	DecryptionKey []byte `hcl:"decryption_key" cty:"decryption_key" json:"decryption_key" yaml:"decryption_key"`
	Metadata      any
}

func (o *Output) Cty() cty.Value {
	return cty.ObjectVal(map[string]cty.Value{
		"encryption_key": o.byteToCty(o.EncryptionKey),
		"decryption_key": o.byteToCty(o.DecryptionKey),
	})
}

func (o *Output) byteToCty(data []byte) cty.Value {
	ctyData := make([]cty.Value, len(data))
	for i, d := range data {
		ctyData[i] = cty.NumberIntVal(int64(d))
	}
	return cty.ListVal(ctyData)
}
