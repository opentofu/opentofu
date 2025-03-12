// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"

	"github.com/hashicorp/hcl/v2"
)

const (
	encryptionVersion = "v0"
)

type baseEncryption struct {
	enc        *encryption
	name       string
	methods    []config.MethodConfig
	encMethod  method.Method
	encMeta    keyProviderMetadata
	staticEval *configs.StaticEvaluator
}

type keyProviderMetamap map[keyprovider.MetaStorageKey][]byte

type keyProviderMetadata struct {
	input  keyProviderMetamap
	output keyProviderMetamap
}

func newBaseEncryption(enc *encryption, target *config.TargetConfig, enforced bool, name string, staticEval *configs.StaticEvaluator) (*baseEncryption, hcl.Diagnostics) {
	// Lookup method configs for the target, ordered by fallback precedence
	methods, diags := methodConfigsFromTarget(enc.cfg, target, name, enforced)
	if diags.HasErrors() {
		return nil, diags
	}

	// Setup the encryptor
	//
	//     Instead of creating new encryption key data for each call to encrypt, we use the same encryptor for the given application (statefile or planfile).
	//
	// Why do we do this?
	//
	//   This allows us to always be in a state where we can encrypt data, which is particularly important when dealing with crashes. If the network is severed
	//   mid-apply, we still need to be able to write an encrypted errored.tfstate or dump to stdout. Additionally it reduces the overhead of encryption in
	//   general, as well as reducing cloud key provider costs.
	//
	// What are the security implications?
	//
	//   Plan file flow is fairly simple and is not impacted by this change. It only ever calls encrypt once at the end of plan generation.
	//
	//   State file is a bit more complex. The encrypted local state file (terraform.tfstate, .terraform.tfstate) will be written with the same
	//   keys as any remote state. These files should be identical, which will make debugging easier.
	//
	//   The major concern with this is that many of the encryption methods used have a limit to how much data a key can safely encrypt. Pbkdf2 for example
	//   has a limit of around 64GB before exhaustion is approached. Writing to the two local and one remote location specified above could not
	//   approach that limit. However the cached state file (.terraform/terraform.tfstate) is persisted every 30 seconds during long applies. For an
	//   extremely large state file (100MB) it would take an apply of over 5 hours to come close to the 64GB limit of pbkdf2 with some malicious actor recording
	//   every single change to the filesystem (or inspecting deleted blocks).
	//
	// What other benefits does this provide?
	//
	//   This performs a e2e validation run of the config -> primary method flow. It serves as a validation step and allows us to return detailed
	//   diagnostics here and simple errors in the decrypt function below (as long as fallback is not used).
	//

	encMeta := keyProviderMetadata{
		input:  make(keyProviderMetamap),
		output: make(keyProviderMetamap),
	}

	// methodConfigsFromTarget guarantees that there will be at least one encryption method.  They are not optional in the common target
	// block, which is required to get to this code.
	encMethod, encDiags := setupMethod(enc.cfg, methods[0], encMeta, enc.reg, staticEval)
	diags = diags.Extend(encDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	base := &baseEncryption{
		enc:        enc,
		name:       name,
		staticEval: staticEval,
		methods:    methods,
		encMethod:  encMethod,
		encMeta:    encMeta,
	}

	return base, diags
}

type basedata struct {
	Meta    keyProviderMetamap `json:"meta"`
	Data    []byte             `json:"encrypted_data"`
	Version string             `json:"encryption_version"` // This is both a sigil for a valid encrypted payload and a future compatibility field
}

func IsEncryptionPayload(data []byte) (bool, error) {
	es := basedata{}
	err := json.Unmarshal(data, &es)
	if err != nil {
		return false, err
	}

	// This could be extended with full version checking later on
	return es.Version != "", nil
}

func (base *baseEncryption) encrypt(data []byte, enhance func(basedata) interface{}) ([]byte, error) {
	encryptor := base.encMethod

	if unencrypted.Is(encryptor) {
		return data, nil
	}

	encd, err := encryptor.Encrypt(data)
	if err != nil {
		return nil, fmt.Errorf("encryption failed for %s: %w", base.name, err)
	}

	es := basedata{
		Version: encryptionVersion,
		Meta:    base.encMeta.output,
		Data:    encd,
	}
	jsond, err := json.Marshal(enhance(es))
	if err != nil {
		return nil, fmt.Errorf("unable to encode encrypted data as json: %w", err)
	}

	return jsond, nil
}

type EncryptionStatus int

const (
	StatusUnknown   EncryptionStatus = 0
	StatusSatisfied EncryptionStatus = 1
	StatusMigration EncryptionStatus = 2
)

// TODO Find a way to make these errors actionable / clear
func (base *baseEncryption) decrypt(data []byte, validator func([]byte) error) ([]byte, EncryptionStatus, error) {
	inputData := basedata{}
	err := json.Unmarshal(data, &inputData)

	if len(inputData.Version) == 0 || err != nil {
		// Not a valid payload, might be already decrypted
		verr := validator(data)
		if verr != nil {
			// Nope, just bad input

			// Return the outer json error if we have one
			if err != nil {
				return nil, StatusUnknown, fmt.Errorf("invalid data format for decryption: %w, %w", err, verr)
			}

			// Must have been invalid json payload
			return nil, StatusUnknown, fmt.Errorf("unable to determine data structure during decryption: %w", verr)
		}

		// Yep, it's already decrypted
		unencryptedSupported := false
		for _, method := range base.methods {
			if unencrypted.IsConfig(method) {
				unencryptedSupported = true
				break
			}
		}
		if !unencryptedSupported {
			return nil, StatusUnknown, fmt.Errorf("encountered unencrypted payload without unencrypted method configured")
		}
		if unencrypted.IsConfig(base.methods[0]) {
			// Decrypted and no pending migration
			return data, StatusSatisfied, nil
		}
		// Decrypted and pending migration
		return data, StatusMigration, nil
	}
	// This is not actually used, only the map inside the Meta parameter is. This is because we are passing the map
	// around.
	outputData := basedata{
		Meta: make(keyProviderMetamap),
	}

	if inputData.Version != encryptionVersion {
		return nil, StatusUnknown, fmt.Errorf("invalid encrypted payload version: %s != %s", inputData.Version, encryptionVersion)
	}

	errs := make([]error, 0)
	for i, method := range base.methods {
		if unencrypted.IsConfig(method) {
			// Not applicable
			continue
		}

		// TODO Discuss if we should potentially cache this based on a json-encoded version of inputData.Meta and reduce overhead dramatically
		decMethod, diags := setupMethod(base.enc.cfg, method, keyProviderMetadata{
			input:  inputData.Meta,
			output: outputData.Meta,
		}, base.enc.reg, base.staticEval)
		if diags.HasErrors() {
			// This cast to error here is safe as we know that at least one error exists
			return nil, StatusUnknown, diags
		}

		uncd, err := decMethod.Decrypt(inputData.Data)
		if err == nil {
			// Success
			if i == 0 {
				// Decrypted with first method (encryption method)
				return uncd, StatusSatisfied, nil
			}
			// Used a fallback
			return uncd, StatusMigration, nil
		}
		// Record the failure
		errs = append(errs, fmt.Errorf("attempted decryption failed for %s: %w", base.name, err))
	}

	// This is good enough for now until we have better/distinct errors
	errMessage := "decryption failed for all provided methods: "
	sep := ""
	for _, err := range errs {
		errMessage += err.Error() + sep
		sep = "\n"
	}
	return nil, StatusUnknown, errors.New(errMessage)
}
