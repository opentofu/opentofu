package encryption

import (
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

type instanceCache struct {
	instances map[string]encryptionflow.Flow
}

var cache *instanceCache

// EnableCaching enables encryption flow instance caching.
//
// This allows code outside this package to call Instance() multiple times with the same key and receive
// the same instance each time.
//
// Similarly, RemoteStateInstance(), StatefileInstance(), or PlanfileInstance() will only construct their
// instance the first time they are called.
//
// Note: you should not enable the instance cache in low level unit tests, but if you do use it in a test,
// you should
//
//	defer DisableCaching()
//
// right after the call to EnableCaching() to clean up after yourself.
func EnableCaching() {
	logging.HCLogger().Trace("enabling state encryption flow instance cache")
	if cache == nil {
		cache = &instanceCache{
			instances: make(map[string]encryptionflow.Flow),
		}
	}
}

// DisableCaching disables encryption flow instance caching.
//
// see EnableCaching().
func DisableCaching() {
	logging.HCLogger().Trace("disabling state encryption flow instance cache")
	cache = nil
}

func (c *instanceCache) cachedOrNewInstance(configKey string, defaultsApply bool) (encryptionflow.Flow, error) {
	instance, found := c.instances[configKey]
	if found {
		logging.HCLogger().Trace("found state encryption flow instance in cache", "configKey", configKey)
		return instance, nil
	}

	instance, err := newInstance(configKey, defaultsApply)
	if err != nil {
		return nil, err
	}

	c.instances[configKey] = instance
	return instance, nil
}
