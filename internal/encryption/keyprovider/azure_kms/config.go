package azure_kms

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
)

type Config struct {
	VaultName    string `hcl:"vault_name"`
	KeyName      string `hcl:"key_name"`
	KeyAlgorithm string `hcl:"key_algorithm"`
	KeySize      int    `hcl:"key_size"`
}

func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	var algo azkeys.EncryptionAlgorithm
	for _, v := range azkeys.PossibleEncryptionAlgorithmValues() {
		if string(v) == c.KeyAlgorithm {
			algo = v
		}
	}
	if len(algo) == 0 {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Message: fmt.Sprintf("expected one of %v as key_algorithm, got %q instead", azkeys.PossibleEncryptionAlgorithmValues(), c.KeyAlgorithm)}
	}

	// TODO validate key_size

	ctx := context.Background()

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Cause: err}
	}

	client, err := azkeys.NewClient(fmt.Sprintf("https://%s.vault.azure.net", c.VaultName), cred, nil)
	if err != nil {
		return nil, nil, &keyprovider.ErrInvalidConfiguration{Cause: err}
	}

	return &keyProvider{
		svc:          client,
		ctx:          ctx,
		keyName:      c.KeyName,
		keyAlgorithm: algo,
		keySize:      c.KeySize,
	}, new(keyMeta), nil
}
