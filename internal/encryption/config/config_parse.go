// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

// DecodeConfig takes a hcl.Body and decodes it into a Config struct.
// This method is here as an example for how someone using this library might want to decode a configuration.
// if they were not using gohcl directly.
// Right now for real world use this is only intended to be used in tests, until we publish this publicly.
func DecodeConfig(body hcl.Body, rng hcl.Range) (*EncryptionConfig, hcl.Diagnostics) {
	cfg := &EncryptionConfig{DeclRange: rng}

	diags := gohcl.DecodeBody(body, nil, cfg)
	if diags.HasErrors() {
		return nil, diags
	}

	for i, kp := range cfg.KeyProviderConfigs {
		for j, okp := range cfg.KeyProviderConfigs {
			if i != j && kp.Type == okp.Type && kp.Name == okp.Name {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate key_provider",
					Detail:   fmt.Sprintf("Found multiple instances of key_provider.%s.%s", kp.Type, kp.Name),
					Subject:  rng.Ptr(),
				})
				break
			}
		}
	}

	for i, m := range cfg.MethodConfigs {
		for j, om := range cfg.MethodConfigs {
			if i != j && m.Type == om.Type && m.Name == om.Name {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate method",
					Detail:   fmt.Sprintf("Found multiple instances of method.%s.%s", m.Type, m.Name),
					Subject:  rng.Ptr(),
				})
				break
			}
		}
	}

	if cfg.Remote != nil {
		for i, t := range cfg.Remote.Targets {
			for j, ot := range cfg.Remote.Targets {
				if i != j && t.Name == ot.Name {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Duplicate remote_data_source",
						Detail:   fmt.Sprintf("Found multiple instances of remote_data_source.%s", t.Name),
						Subject:  rng.Ptr(),
					})
					break
				}
			}
		}
	}

	if cfg.Plan != nil {
		if cfg.Plan.MigrateToUnencrypted && cfg.Plan.MigrateToEncrypted {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Field conflict in encryption",
				Detail:   "Only one of migrate_to_encrypted or migrate_to_unencrypted may be specified",
				Subject:  rng.Ptr(),
			})
		} else {
			if cfg.Plan.MigrateToUnencrypted {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagWarning,
					Summary:  "Plan Encryption Migration Enabled",
					Detail:   "plan > migrate_to_unencrypted is enabled and should be disabled once the migration is complete",
					Subject:  rng.Ptr(),
				})
			}
			if cfg.Plan.MigrateToEncrypted {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagWarning,
					Summary:  "Plan Encryption Migration Enabled",
					Detail:   "plan > migrate_to_encrypted is enabled and should be disabled once the migration is complete",
					Subject:  rng.Ptr(),
				})
			}
		}
	}
	if cfg.State != nil {
		if cfg.State.MigrateToUnencrypted && cfg.State.MigrateToEncrypted {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Field conflict in encryption",
				Detail:   "Only one of migrate_to_encrypted or migrate_to_unencrypted may be specified",
				Subject:  rng.Ptr(),
			})
		} else {
			if cfg.State.MigrateToUnencrypted {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagWarning,
					Summary:  "State Encryption Migration Enabled",
					Detail:   "plan > migrate_to_unencrypted is enabled and should be disabled once the migration is complete",
					Subject:  rng.Ptr(),
				})
			}
			if cfg.State.MigrateToEncrypted {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagWarning,
					Summary:  "State Encryption Migration Enabled",
					Detail:   "plan > migrate_to_encrypted is enabled and should be disabled once the migration is complete",
					Subject:  rng.Ptr(),
				})
			}
		}
	}

	if diags.HasErrors() {
		return nil, diags
	}

	return cfg, diags
}
