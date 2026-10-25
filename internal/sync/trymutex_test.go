package sync

import (
	"testing"
	"time"
)

func TestTryMutexBasicLocking(t *testing.T) {
	var tm TryMutex

	tm.Lock()
	if tm.TryLock() {
		t.Fatal("TryLock succeeded while the mutex was held")
	}
	tm.Unlock()

	if !tm.TryLock() {
		t.Fatal("TryLock failed on an unlocked mutex")
	}
	tm.Unlock()
}

func TestTryMutexLockBlocks(t *testing.T) {
	var tm TryMutex

	tm.Lock()
	acquired := make(chan struct{})
	go func() {
		tm.Lock()
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("Lock acquired while the mutex was held")
	case <-time.After(20 * time.Millisecond):
	}

	tm.Unlock()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("Lock not acquired after Unlock")
	}
	tm.Unlock()
}

func TestTryMutexTryLockTimed(t *testing.T) {
	var tm TryMutex

	// Zero timeout on an unlocked mutex must still succeed.
	if !tm.TryLockTimed(0) {
		t.Fatal("TryLockTimed(0) failed on an unlocked mutex")
	}

	// Held mutex: times out.
	start := time.Now()
	if tm.TryLockTimed(30 * time.Millisecond) {
		t.Fatal("TryLockTimed succeeded while the mutex was held")
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Fatalf("TryLockTimed returned after %v, want at least 30ms", elapsed)
	}

	// Released during the wait: succeeds.
	go func() {
		time.Sleep(20 * time.Millisecond)
		tm.Unlock()
	}()
	if !tm.TryLockTimed(time.Second) {
		t.Fatal("TryLockTimed failed although the mutex was released during the wait")
	}
	tm.Unlock()
}

func TestTryMutexUnlockPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Unlock of an unlocked TryMutex did not panic")
		}
	}()
	var tm TryMutex
	tm.Unlock()
}
