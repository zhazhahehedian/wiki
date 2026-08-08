package service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestKeyedMutexSerializesAndCleansUpEntries(t *testing.T) {
	locks := newKeyedMutex()
	start := make(chan struct{})
	var wg sync.WaitGroup
	var active atomic.Int32
	var overlaps atomic.Int32

	const callers = 32
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			unlock := locks.Lock("account-1")
			if active.Add(1) != 1 {
				overlaps.Add(1)
			}
			time.Sleep(time.Millisecond)
			active.Add(-1)
			unlock()
		}()
	}
	close(start)
	wg.Wait()

	if overlaps.Load() != 0 {
		t.Fatalf("same-key critical sections overlapped %d times", overlaps.Load())
	}
	locks.mu.Lock()
	remaining := len(locks.entries)
	locks.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("keyed lock entries = %d, want 0", remaining)
	}
}
