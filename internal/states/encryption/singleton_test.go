package encryption

import "testing"

func TestGetSingleton(t *testing.T) {
	t.Run("singleton", func(t *testing.T) {

		t.Cleanup(ClearSingleton)
		GetSingleton() // MUST be used in conjunction with t.Cleanup(ClearSingleton) in tests

		if instance == nil {
			t.Fatalf("GetSingleton did not create the singleton, it was still nil")
		}
	})
	t.Run("cleanup", func(t *testing.T) {
		if instance != nil {
			t.Errorf("ClearSingleton did not remove the singleton, it was still non-nil")
		}
	})

}
