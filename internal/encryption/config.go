package encryption

import (
	"github.com/hashicorp/hcl/v2"
)

type Config struct {
	KeyProviderConfigs []KeyProviderConfig `hcl:"key_provider,block"`
	MethodConfigs      []MethodConfig      `hcl:"method,block"`

	Backend   *EnforcableTargetConfig `hcl:"backend,block"`
	StateFile *EnforcableTargetConfig `hcl:"statefile,block"`
	PlanFile  *EnforcableTargetConfig `hcl:"planfile,block"`
	Remote    *RemoteConfig           `hcl:"remote_data_source,block"`
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

type RemoteConfig struct {
	Default *TargetConfig       `hcl:"default,block"`
	Targets []NamedTargetConfig `hcl:"remote_data_source,block"`
}

// gohcl does not support struct embedding (yet)
type TargetConfig struct {
	Method   hcl.Expression `hcl:"method,optional"`
	Fallback *TargetConfig  `hcl:"fallback,block"`
}

type EnforcableTargetConfig struct {
	Enforced bool           `hcl:"enforced,optional"`
	Method   hcl.Expression `hcl:"method,optional"`
	Fallback *TargetConfig  `hcl:"fallback,block"`
}

func (e EnforcableTargetConfig) AsTargetConfig() *TargetConfig {
	return &TargetConfig{
		Method:   e.Method,
		Fallback: e.Fallback,
	}
}

type NamedTargetConfig struct {
	Name     string         `hcl:"name,label"`
	Method   hcl.Expression `hcl:"method,optional"`
	Fallback *TargetConfig  `hcl:"fallback,block"`
}

func (n NamedTargetConfig) AsTargetConfig() *TargetConfig {
	return &TargetConfig{
		Method:   n.Method,
		Fallback: n.Fallback,
	}
}
