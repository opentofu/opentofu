package keyprovider

import "github.com/zclconf/go-cty/cty"

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
