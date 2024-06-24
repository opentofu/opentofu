// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption/config"
)

// PlanEncryption describes the methods that you can use for encrypting a plan file. Plan files are opaque values with
// no standardized format, so the encrypted form should be treated equally an opaque value.
type PlanEncryption interface {
	// EncryptPlan encrypts a plan file and returns the encrypted form.
	//
	// When implementing this function:
	//
	// Plan files are opaque values. You may expect a valid plan file on the input, but you can produce binary data
	// that is not necessarily a valid plan file. If no encryption is configured, this function should pass through
	// any data it receives without modification, even if the plan file is invalid.
	//
	// When using this function:
	//
	// Make sure that you pass a valid plan file as an input. Failing to provide a valid plan file may result in an
	// error. However, output values may not be valid plan files and you should not pass the encrypted plan file to any
	// additional functions that normally work with plan files.
	EncryptPlan([]byte) ([]byte, error)

	// DecryptPlan decrypts an encrypted plan file.
	//
	// When implementing this function:
	//
	// If the user has configured no encryption, pass through any input unmodified regardless if the input is a valid
	// plan file. If the user configured encryption, decrypt the plan file and return the decrypted plan file as a
	// binary without further evaluating its validity.
	//
	// When using this function:
	//
	// Pass a potentially encrypted plan file as an input, and you will receive the decrypted plan file or an error as
	// a result.
	DecryptPlan([]byte) ([]byte, error)
}

type planEncryption struct {
	base *baseEncryption
}

func newPlanEncryption(enc *encryption, target *config.TargetConfig, enforced bool, name string, staticEval *configs.StaticEvaluator) (PlanEncryption, hcl.Diagnostics) {
	base, diags := newBaseEncryption(enc, target, enforced, name, staticEval)
	return &planEncryption{base}, diags
}

func (p planEncryption) EncryptPlan(data []byte) ([]byte, error) {
	return p.base.encrypt(data, func(base basedata) interface{} { return base })
}

func (p planEncryption) DecryptPlan(data []byte) ([]byte, error) {
	return p.base.decrypt(data, func(data []byte) error {
		// Check magic bytes
		if len(data) < 2 || string(data[:2]) != "PK" {
			return fmt.Errorf("Invalid plan file %v", string(data[:2]))
		}
		return nil
	})
}

func PlanEncryptionDisabled() PlanEncryption {
	return &planDisabled{}
}

type planDisabled struct{}

func (s *planDisabled) EncryptPlan(plainPlan []byte) ([]byte, error) {
	return plainPlan, nil
}
func (s *planDisabled) DecryptPlan(encryptedPlan []byte) ([]byte, error) {
	return encryptedPlan, nil
}
