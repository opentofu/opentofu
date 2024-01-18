package encryptionflow_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

func ExampleFlow() {
	var builder encryptionflow.FlowBuilder = &base64UnsafeFlowBuilder{}

	flow, err := builder.Build()
	if err != nil {
		panic(err)
	}
	encryptedState, err := flow.EncryptState([]byte(`{"message":"Hello world!"}`))
	if err != nil {
		panic(err)
	}

	decryptedState, err := flow.DecryptState(encryptedState)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s\n", decryptedState)

	//Output: {"message":"Hello world!"}
}

type base64UnsafeFlowBuilder struct{}

func (b base64UnsafeFlowBuilder) EncryptionConfiguration(source encryptionflow.ConfigurationSource, config encryptionconfig.Config) error {
	return nil
}

func (b base64UnsafeFlowBuilder) DecryptionFallbackConfiguration(source encryptionflow.ConfigurationSource, config encryptionconfig.Config) error {
	return nil
}

func (b base64UnsafeFlowBuilder) Build() (encryptionflow.Flow, error) {
	return &base64UnsafeFlow{}, nil
}

type base64UnsafeFlow struct {
}

type encryptedState struct {
	Data string `json:"data"`
}

func (b base64UnsafeFlow) DecryptState(payload []byte) ([]byte, error) {
	return b.decrypt(payload)
}

func (b base64UnsafeFlow) EncryptState(state []byte) ([]byte, error) {
	return b.encrypt(state)
}

func (b base64UnsafeFlow) DecryptPlan(payload []byte) ([]byte, error) {
	return b.decrypt(payload)
}

func (b base64UnsafeFlow) EncryptPlan(plan []byte) ([]byte, error) {
	return b.encrypt(plan)
}

func (b base64UnsafeFlow) decrypt(payload []byte) ([]byte, error) {
	var unmarshalledPayload *encryptedState
	if err := json.Unmarshal(payload, &unmarshalledPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal encrypted data (%w)", err)
	}

	// Decode the data.
	decodedData, err := base64.StdEncoding.DecodeString(unmarshalledPayload.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode encrypted payload (%w)", err)
	}

	// Make sure there's valid JSON in the decrypted data.
	var result any
	if err := json.Unmarshal(decodedData, &result); err != nil {
		return nil, fmt.Errorf("failed to decode decrypted data (%w)", err)
	}
	return decodedData, nil
}

func (b base64UnsafeFlow) encrypt(payload []byte) ([]byte, error) {
	var testResult any
	if err := json.Unmarshal(payload, &testResult); err != nil {
		return nil, fmt.Errorf("failed to decode decrypted data (%w)", err)
	}

	encryptedPayload := &encryptedState{
		base64.StdEncoding.EncodeToString(payload),
	}
	result, err := json.Marshal(encryptedPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to JSON-encode encrypted payload (%w)", err)
	}
	return result, nil
}
