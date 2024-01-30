package encryption

import (
	"github.com/hashicorp/hcl/v2"
)

type Config struct {
	KeyProviders map[string]hcl.Body
	Methods      map[string]hcl.Body

	Targets       map[string]*TargetConfig
	RemoteTargets *RemoteTargetsConfig

	// Used to identify conflicting blocks in the same module
	DeclRange hcl.Range
}

type TargetConfig struct {
	Enforced bool
	Method   string
	Fallback *TargetConfig
}

type RemoteTargetsConfig struct {
	Default *TargetConfig
	Targets map[string]*TargetConfig
}
