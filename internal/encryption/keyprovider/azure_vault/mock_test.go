package azure_vault

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/opentofu/opentofu/internal/backend/remote-state/assure/auth"
	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/tracing"
)

type mockKMC struct {
	encrypt func(azkeys.KeyOperationParameters) (azkeys.EncryptResponse, error)
	decrypt func(azkeys.KeyOperationParameters) (azkeys.DecryptResponse, error)
}

func (m *mockKMC) Decrypt(_ context.Context, _ string, _ string, parameters azkeys.KeyOperationParameters, _ *azkeys.DecryptOptions) (azkeys.DecryptResponse, error) {
	return m.decrypt(parameters)
}

func (m *mockKMC) Encrypt(_ context.Context, _ string, _ string, parameters azkeys.KeyOperationParameters, _ *azkeys.EncryptOptions) (azkeys.EncryptResponse, error) {
	return m.encrypt(parameters)
}

type mockPager struct {
	called bool
	runtime.PagingHandler[azkeys.ListKeyPropertiesVersionsResponse]
}

func (m *mockPager) init() {
	// This is only used to create a pointer.
	tempBool := true
	mockId := azkeys.ID("https://myvaultname.vault.azure.net/keys/key1053998307/b86c2e6ad9054f4abf69cc185b99aa60")
	m.PagingHandler = runtime.PagingHandler[azkeys.ListKeyPropertiesVersionsResponse]{
		More: func(azkeys.ListKeyPropertiesVersionsResponse) bool {
			return !m.called
		},
		Fetcher: func(context.Context, *azkeys.ListKeyPropertiesVersionsResponse) (azkeys.ListKeyPropertiesVersionsResponse, error) {
			m.called = true
			return azkeys.ListKeyPropertiesVersionsResponse{
				KeyPropertiesListResult: azkeys.KeyPropertiesListResult{
					Value: []*azkeys.KeyProperties{{
						Attributes: &azkeys.KeyAttributes{Enabled: &tempBool},
						KID:        &mockId,
					}},
				},
			}, nil
		},
		Tracer: tracing.Tracer{},
	}
}

func (m *mockKMC) NewListKeyPropertiesVersionsPager(name string, options *azkeys.ListKeyPropertiesVersionsOptions) *runtime.Pager[azkeys.ListKeyPropertiesVersionsResponse] {
	mp := mockPager{}
	mp.init()
	return runtime.NewPager(mp.PagingHandler)
}

func (m *mockKMC) Construct(ctx context.Context, config *auth.Config) (azcore.TokenCredential, error) {
	return nil, nil
}

func (m *mockKMC) Validate(config *auth.Config) tfdiags.Diagnostics {
	return nil
}

func (m *mockKMC) AugmentConfig(config *auth.Config) error {
	return nil
}

func injectMock(m *mockKMC) {
	newKeyManagementClient = func(_ string, _ azcore.TokenCredential) (keyManagementClient, error) {
		return m, nil
	}
	getAuthMethod = func(_ *auth.Config) (auth.AuthMethod, error) {
		return m, nil
	}
}
