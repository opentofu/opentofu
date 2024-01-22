package encryption

import "testing"

func TestGetSingleton(t *testing.T) {
	GetSingleton() // MUST be followed by defer DisableCaching() in tests
	defer func() {
		if instance != nil {
			t.Errorf("ClearSingleton did not remove the singleton, it was still non-nil")
		}
	}()
	defer ClearSingleton()

	if instance == nil {
		t.Fatalf("GetSingleton did not create the singleton, it was still nil")
	}
}

/*
func TestCachedOrNewInstance(t *testing.T) {
	EnableSingletonCaching() // MUST be followed by defer DisableCaching() in tests
	defer DisableSingletonCaching()

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
*/
