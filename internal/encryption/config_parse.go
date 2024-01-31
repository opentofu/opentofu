package encryption

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

func DecodeConfig(body hcl.Body, rng hcl.Range) (*Config, hcl.Diagnostics) {
	cfg := &Config{}

	diags := gohcl.DecodeBody(body, nil, cfg)
	if diags.HasErrors() {
		return nil, diags
	}

	for i, kp := range cfg.KeyProviders {
		for j, okp := range cfg.KeyProviders {
			if i != j && kp.Type == okp.Type && kp.Name == okp.Name {
				panic("TODO diags")
				break
			}
		}
	}

	for i, m := range cfg.Methods {
		for j, om := range cfg.Methods {
			if i != j && m.Type == om.Type && m.Name == om.Name {
				panic("TODO diags")
				break
			}
		}
	}

	if cfg.Remote != nil {
		for i, t := range cfg.Remote.Targets {
			for j, ot := range cfg.Remote.Targets {
				if i != j && t.Name == ot.Name {
					panic("TODO diags")
					break
				}
			}
		}
	}

	return cfg, diags
}
