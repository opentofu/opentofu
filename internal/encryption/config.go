package encryption

import (
	"github.com/hashicorp/hcl/v2"
)

type Config struct {
	KeyProviders []KeyProviderConfig `hcl:"key_provider,block"`
	Methods      []MethodConfig      `hcl:"method,block"`

	Backend   *TargetConfig `hcl:"backend,block"`
	StateFile *TargetConfig `hcl:"statefile,block"`
	PlanFile  *TargetConfig `hcl:"planfile,block"`
	Remote    *RemoteConfig `hcl:"remote,block"`
}

type KeyProviderConfig struct {
	Type string   `hcl:"type,label"`
	Name string   `hcl:"name,label"`
	Body hcl.Body `hcl:",remain"`
}

type MethodConfig struct {
	Type string   `hcl:"type,label"`
	Name string   `hcl:"name,label"`
	Body hcl.Body `hcl:",remain"`
}

type TargetConfig struct {
	Enforced bool           `hcl:"enforced,optional"`
	Method   hcl.Expression `hcl:"method,optional"`
	Fallback *TargetConfig  `hcl:"fallback,block"`
}

type RemoteConfig struct {
	Default *TargetConfig        `hcl:"default,block"`
	Targets []RemoteTargetConfig `hcl:"remote_data_source,block"`
}

type RemoteTargetConfig struct {
	Name string `hcl:"name,label"`
	// gohcl does not support struct embedding
	Enforced bool           `hcl:"enforced,optional"`
	Method   hcl.Expression `hcl:"method,optional"`
	Fallback *TargetConfig  `hcl:"fallback,block"`
}

func (r RemoteTargetConfig) AsTargetConfig() *TargetConfig {
	return &TargetConfig{
		Enforced: r.Enforced,
		Method:   r.Method,
		Fallback: r.Fallback,
	}
}
