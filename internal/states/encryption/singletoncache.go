package encryption

import (
	"sync"

	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

// IMPORTANT: we cache singletons in this package!
//
// please read the note at the top of instance.go for details

type instanceCache struct {
	instances_useGetAndSet map[string]encryptionflow.Flow
	mutex                  sync.RWMutex
}

var cache *instanceCache

// EnableSingletonCaching enables encryption flow instance caching.
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
//	defer DisableSingletonCaching()
//
// right after the call to EnableSingletonCaching() to clean up after yourself.
func EnableSingletonCaching() {
	logging.HCLogger().Trace("enabling state encryption flow singleton instance cache")
	if cache == nil {
		cache = &instanceCache{
			instances_useGetAndSet: make(map[string]encryptionflow.Flow),
		}
	}
}

// DisableSingletonCaching disables encryption flow instance caching.
//
// see EnableSingletonCaching().
func DisableSingletonCaching() {
	logging.HCLogger().Trace("disabling state encryption flow singleton instance cache")
	cache = nil
}

func (c *instanceCache) get(configKey string) (cacheEntry encryptionflow.Flow, ok bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	cacheEntry, ok = c.instances_useGetAndSet[configKey]
	return
}

func (c *instanceCache) set(configKey string, cacheEntry encryptionflow.Flow) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.instances_useGetAndSet[configKey] = cacheEntry
}

func (c *instanceCache) cachedOrNewInstance(configKey string, defaultsApply bool) (encryptionflow.Flow, error) {
	instance, found := c.get(configKey)
	if found {
		logging.HCLogger().Trace("found state encryption flow singleton instance in cache", "configKey", configKey)
		return instance, nil
	}

	instance, err := newInstance(configKey, defaultsApply)
	if err != nil {
		return nil, err
	}

	c.set(configKey, instance)
	return instance, nil
}
