package encryption

import "testing"

func TestEnableDisableCaching(t *testing.T) {
	EnableCaching() // MUST be followed by defer DisableCaching() in tests
	defer func() {
		if cache != nil {
			t.Errorf("DisableCaching did not remove the cache, it was still non-nil")
		}
	}()
	defer DisableCaching()

	if cache == nil {
		t.Fatalf("EnableCaching did not create the cache, it was still nil")
	}
}

func TestCachedOrNewInstance(t *testing.T) {
	EnableCaching() // MUST be followed by defer DisableCaching() in tests
	defer DisableCaching()

	const configKey = "unit_testing.test_cached_or_new[1]"

	instance, err := cache.cachedOrNewInstance(configKey, false)
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}
	if instance == nil {
		t.Errorf("instance on initial creation was unexpectedly nil")
	}

	if len(cache.instances) == 0 {
		t.Error("instance was not cached after creation")
	}

	instance, err = cache.cachedOrNewInstance(configKey, false)
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}
	if instance == nil {
		t.Errorf("instance on cache retrieval was unexpectedly nil")
	}
}
