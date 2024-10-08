package states

import (
	"fmt"
	"github.com/opentofu/opentofu/internal/addrs"
	"testing"
)

func TestModule_SetResourceInstanceCurrent_setProviders(t *testing.T) {
	var (
		resourceInstance, _ = addrs.ParseAbsResourceInstanceStr("null_resource.resource")
		mockAddr            = resourceInstance.Resource
		provider, _         = addrs.ParseAbsProviderConfigStr("provider[\"registry.opentofu.org/hashicorp/aws\"]")
		mockObject          = &ResourceInstanceObjectSrc{
			SchemaVersion: 1,
			AttrsJSON:     []byte(`{"hello": "world"}`),
		}
	)

	module := &Module{
		Resources: make(map[string]*Resource),
	}

	t.Run("valid - resourceProvider set, instanceProvider not set", func(t *testing.T) {
		module.Resources = make(map[string]*Resource)
		module.SetResourceInstanceCurrent(mockAddr, mockObject, provider, addrs.AbsProviderConfig{})

		resource := module.Resources[mockAddr.Resource.String()]
		if resource == nil {
			t.Errorf("Expected resource to be created")
		} else if !resource.ProviderConfig.IsSet() || resource.ProviderConfig.String() != provider.String() {
			t.Errorf("Expected resourceProvider to be set correctly, got %+v", resource.ProviderConfig)
		}

		if mockObject.InstanceProvider.IsSet() {
			t.Errorf("Expected instanceProvider to be not set, got %+v", mockObject.InstanceProvider)
		}
	})

	t.Run("valid - instanceProvider set, resourceProvider not set", func(t *testing.T) {
		module.Resources = make(map[string]*Resource)
		module.SetResourceInstanceCurrent(mockAddr, mockObject, addrs.AbsProviderConfig{}, provider)

		resource := module.Resources[mockAddr.Resource.String()]
		if resource == nil {
			t.Errorf("Expected resource to be created")
		} else if resource.ProviderConfig.IsSet() {
			t.Errorf("Expected resourceProvider to be not set, got %+v", resource.ProviderConfig)
		}

		if !mockObject.InstanceProvider.IsSet() || mockObject.InstanceProvider.String() != provider.String() {
			t.Errorf("Expected instanceProvider to be set correctly, got %+v", mockObject.InstanceProvider)
		}
	})

	t.Run("both providers set should panic", func(t *testing.T) {
		module.Resources = make(map[string]*Resource)
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic when both providers are set")
			} else if r != fmt.Sprintf("SetResourceInstanceCurrent for %s (instance key nil) got two providers (resourceProvider & instanceProvider) to write in state", mockAddr.Resource) {
				t.Errorf("Unexpected panic message: got %v", r)
			}
		}()
		module.SetResourceInstanceCurrent(mockAddr, mockObject, provider, provider)
	})

	t.Run("neither provider set should panic", func(t *testing.T) {
		module.Resources = make(map[string]*Resource)
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic when neither provider is set")
			} else if r != fmt.Sprintf("SetResourceInstanceCurrent for %s (instance key nil) cannot find a provider (resourceProvider / instanceProvider) to write in state", mockAddr.Resource) {
				t.Errorf("Unexpected panic message: got %v", r)
			}
		}()
		module.SetResourceInstanceCurrent(mockAddr, mockObject, addrs.AbsProviderConfig{}, addrs.AbsProviderConfig{})
	})
}

func TestModule_SetResourceInstanceDeposed_setProviders(t *testing.T) {
	var (
		resourceInstance, _ = addrs.ParseAbsResourceInstanceStr("null_resource.resource")
		mockAddr            = resourceInstance.Resource
		provider, _         = addrs.ParseAbsProviderConfigStr("provider[\"registry.opentofu.org/hashicorp/aws\"]")
		mockObject          = &ResourceInstanceObjectSrc{
			SchemaVersion: 1,
			AttrsJSON:     []byte(`{"hello": "world"}`),
		}
	)

	module := &Module{
		Resources: make(map[string]*Resource),
	}

	t.Run("valid - resourceProvider set, instanceProvider not set", func(t *testing.T) {
		module.Resources = make(map[string]*Resource)
		module.SetResourceInstanceDeposed(mockAddr, "deposedKey", mockObject, provider, addrs.AbsProviderConfig{})

		resource := module.Resources[mockAddr.Resource.String()]
		if resource == nil {
			t.Errorf("Expected resource to be created")
		} else if !resource.ProviderConfig.IsSet() || resource.ProviderConfig.String() != provider.String() {
			t.Errorf("Expected resourceProvider to be set correctly, got %+v", resource.ProviderConfig)
		}

		if mockObject.InstanceProvider.IsSet() {
			t.Errorf("Expected instanceProvider to be not set, got %+v", mockObject.InstanceProvider)
		}
	})

	t.Run("valid - instanceProvider set, resourceProvider not set", func(t *testing.T) {
		module.Resources = make(map[string]*Resource)
		module.SetResourceInstanceDeposed(mockAddr, "deposedKey", mockObject, addrs.AbsProviderConfig{}, provider)

		resource := module.Resources[mockAddr.Resource.String()]
		if resource == nil {
			t.Errorf("Expected resource to be created")
		} else if resource.ProviderConfig.IsSet() {
			t.Errorf("Expected resourceProvider to be not set, got %+v", resource.ProviderConfig)
		}

		if !mockObject.InstanceProvider.IsSet() || mockObject.InstanceProvider.String() != provider.String() {
			t.Errorf("Expected instanceProvider to be set correctly, got %+v", mockObject.InstanceProvider)
		}
	})

	t.Run("both providers set should panic", func(t *testing.T) {
		module.Resources = make(map[string]*Resource)
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic when both providers are set")
			} else if r != fmt.Sprintf("SetResourceInstanceDeposed for %s (instance key nil, deposed key deposedKey) got two providers (resourceProvider & instanceProvider) to write in state", mockAddr.Resource) {
				t.Errorf("Unexpected panic message: got %v", r)
			}
		}()
		module.SetResourceInstanceDeposed(mockAddr, "deposedKey", mockObject, provider, provider)
	})

	t.Run("neither provider set should panic", func(t *testing.T) {
		module.Resources = make(map[string]*Resource)
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic when neither provider is set")
			} else if r != fmt.Sprintf("SetResourceInstanceDeposed for %s (instance key nil, deposed key deposedKey) cannot find a provider (resourceProvider / instanceProvider) to write in state", mockAddr.Resource) {
				t.Errorf("Unexpected panic message: got %v", r)
			}
		}()
		module.SetResourceInstanceDeposed(mockAddr, "deposedKey", mockObject, addrs.AbsProviderConfig{}, addrs.AbsProviderConfig{})
	})
}
