// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package flock

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestLock_BasicFunctionality(t *testing.T) {
	// Tests Lock and Unlock for a single file

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "test.lock")

	// Create test file
	f, err := os.Create(lockFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer f.Close()

	// Test acquiring lock
	err = Lock(f)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Test unlocking
	err = Unlock(f)
	if err != nil {
		t.Fatalf("Failed to unlock: %v", err)
	}
}

func TestLock_Contention(t *testing.T) {
	// Tests back-to-back Lock calls on a single file

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "contention.lock")

	// Create test file
	f1, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer f1.Close()

	// Open same file with different handle to simulate multiple accessors
	f2, err := os.OpenFile(lockFile, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to open test file: %v", err)
	}
	defer f2.Close()

	// First lock should always succeed regardless of OS
	err = Lock(f1)
	if err != nil {
		t.Fatalf("First lock should succeed: %v", err)
	}

	// Second lock behavior is OS-dependent due to different locking semantics:
	//
	// UNIX/Linux/macOS (fcntl-based):
	//   - fcntl locks are process-scoped, not file-descriptor-scoped
	//   - Multiple file descriptors in the same process can "share" the lock
	//   - The lock prevents OTHER PROCESSES from acquiring it, not other FDs in same process
	//   - This is POSIX-compliant behavior and intentional for flexibility
	//   - Real contention only occurs between different processes
	//
	// Windows (LockFileEx-based):
	//   - Locks are more granular and can be per-handle depending on flags
	//   - LOCKFILE_EXCLUSIVE_LOCK with different handles may behave differently
	//   - Behavior can vary based on how the file was opened and lock flags used
	//
	// Network File Systems (NFS/CIFS):
	//   - Lock behavior depends on server implementation and mount options
	//   - Some NFS versions don't support fcntl locks properly
	//   - CIFS locking can have different semantics than local filesystems
	err = Lock(f2)
	if err != nil {
		// Case 1: True contention detected (expected on some systems or configurations)
		t.Logf("Second lock failed as expected - true contention detected: %v", err)
		
		// Verify that unlocking the first handle allows the second to succeed
		// We are likely in a Windows OS now and our Unlock implementation for Windows is a no-op
		// As commented there, we need to use Close
		err = f1.Close()
		if err != nil {
			t.Fatalf("Failed to close the first file: %v", err)
		}

		// After releasing first lock, second should be able to acquire it
		err = Lock(f2)
		if err != nil {
			t.Fatalf("Second lock should succeed after first unlock: %v", err)
		}

		err = Unlock(f2)
		if err != nil {
			t.Fatalf("Failed to unlock second handle: %v", err)
		}
	} else {
		// Case 2: Same-process lock sharing (common on POSIX systems)
		t.Logf("Second lock succeeded - same process can hold multiple locks (POSIX behavior)")
		
		// Both handles now "hold" the lock from the OS perspective
		// This is correct behavior for fcntl locks - they're process-scoped
		// The actual protection is against OTHER PROCESSES, not other handles in same process
		
		// Clean up both locks (order doesn't matter for same-process locks)
		err = Unlock(f1)
		if err != nil {
			t.Logf("Unlock f1 returned: %v (may be no-op on some systems)", err)
		}
		
		err = Unlock(f2)
		if err != nil {
			t.Logf("Unlock f2 returned: %v (may be no-op on some systems)", err)
		}
	}
}

func TestLockBlocking_Success(t *testing.T) {
	// Tests LockBlocking and Unlock on a single file

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "blocking.lock")

	// Create test file
	f, err := os.Create(lockFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer f.Close()

	ctx := context.Background()

	// Test blocking lock acquisition
	err = LockBlocking(ctx, f)
	if err != nil {
		t.Fatalf("Failed to acquire blocking lock: %v", err)
	}

	// Test unlocking
	err = Unlock(f)
	if err != nil {
		t.Fatalf("Failed to unlock: %v", err)
	}
}

func TestLockBlocking_Cancellation(t *testing.T) {
	// Tests cancellation of LockBlocking on a single file while a Lock is already in place
	// Doesn't really test anything in POSIX systems since the same process can hold multiple locks
	// on the same file

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "cancel.lock")
	
	// Create test file
	f1, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer f1.Close()

	f2, err := os.OpenFile(lockFile, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to open test file with a second handler: %v", err)
	}
	defer f2.Close()

	// First, test if this system supports real contention between same-process handles
	err = Lock(f1)
	if err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}
	
	// Test if second lock fails
	testErr := Lock(f2)
	if testErr == nil {
		// Same process can acquire multiple locks - POSIX behavior
		t.Logf("System allows same-process multiple locks (POSIX fcntl behavior)")
		t.Logf("Skipping cancellation test - no contention between same-process handles")
		// Not checking return value here since any problem would already be highlighted by
		// TestLock_BasicFunctionality
		_ = Unlock(f1)
		_ = Unlock(f2)
		t.Skip("No contention between same-process handles on this system")
		return
	}
	
	// We have real contention - Windows behaviour, so test cancellation
	t.Logf("System supports real contention between same-process handles")

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Attempt blocking lock with second handle - should be cancelled
	start := time.Now()
	err = LockBlocking(ctx, f2)
	elapsed := time.Since(start)

	// Clean up first lock
	_ = Unlock(f1)

	// Should fail due to timeout or cancellation
	if err == nil {
		t.Fatal("Expected blocking lock to be cancelled, but it succeeded")
	}

	// Check cancellation behavior
	if err == context.DeadlineExceeded || err == context.Canceled {
		t.Logf("Lock was properly cancelled: %v (took %v)", err, elapsed)
		
		// Should have been cancelled within reasonable time
		if elapsed > 200*time.Millisecond {
			t.Fatalf("Cancellation took too long: %v", elapsed)
		}
	} else {
		t.Fatalf("Expected timeout/cancellation error, got: %v", err)
	}
}

func TestLockBlocking_EventualSuccess(t *testing.T) {
	// Tests eventual success of LockBlocking on a single file while a Lock is 
	// already in place which is then released
	// Doesn't really test anything in POSIX systems since the same process can hold multiple locks
	// on the same file

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "eventual.lock")

	// Create test file
	f1, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer f1.Close()

	f2, err := os.OpenFile(lockFile, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to open test file with a second handler: %v", err)
	}
	defer f2.Close()

	// Acquire lock with first handle
	err = Lock(f1)
	if err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}

	var wg sync.WaitGroup
	var lockErr error

	// Start blocking lock in goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx := context.Background()
		lockErr = LockBlocking(ctx, f2)
	}()

	// Release first lock after short delay
	time.Sleep(50 * time.Millisecond)
	if runtime.GOOS == "windows" {
		err = f1.Close()
	} else {
		err = Unlock(f1)
	}
	if err != nil {
		t.Fatalf("Failed to unlock first: %v", err)
	}

	// Wait for blocking lock to succeed
	wg.Wait()

	if lockErr != nil {
		t.Fatalf("Blocking lock should have succeeded: %v", lockErr)
	}

	// Clean up
	err = Unlock(f2)
	if err != nil {
		t.Fatalf("Failed to unlock second: %v", err)
	}
}

func TestConcurrentLocking(t *testing.T) {
	// Tests multiple goroutines simultaneously trying to acquire locks on a single file

	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "concurrent.lock")

	const numGoroutines = 10
	const iterations = 5

	// Create test file
	testFile, err := os.Create(lockFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	testFile.Close()

	var wg sync.WaitGroup
	successCount := make(chan int, numGoroutines)
	
	// Launch multiple goroutines trying to acquire locks
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			successes := 0
			for j := 0; j < iterations; j++ {
				f, err := os.OpenFile(lockFile, os.O_RDWR, 0644)
				if err != nil {
					t.Errorf("Goroutine %d: Failed to open file: %v", id, err)
					continue
				}
				
				err = Lock(f)
				if err == nil {
					successes++
					// Hold lock briefly
					time.Sleep(1 * time.Millisecond)
					_ = Unlock(f)
				}
				f.Close()
				
				// Brief pause between attempts
				time.Sleep(1 * time.Millisecond)
			}
			successCount <- successes
		}(i)
	}

	wg.Wait()
	close(successCount)

	// Count total successes
	totalSuccesses := 0
	for count := range successCount {
		totalSuccesses += count
	}

	// Should have some successes, but not necessarily all attempts
	if totalSuccesses == 0 {
		t.Fatal("No goroutine managed to acquire any locks")
	}

	t.Logf("Total successful lock acquisitions: %d out of %d attempts", 
		totalSuccesses, numGoroutines*iterations)
}
