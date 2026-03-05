package bot

import (
	"sync"
	"testing"
	"time"
)

// newTestBot creates a minimal Bot for testing process control (no Telegram API).
func newTestBot() *Bot {
	return &Bot{
		inflight: make(map[int64]inflightEntry),
	}
}

func TestStartInflight_CreatesContext(t *testing.T) {
	b := newTestBot()
	ctx, cleanup := b.startInflight(42)
	defer cleanup()

	if ctx.Err() != nil {
		t.Fatal("context should not be cancelled immediately")
	}

	b.inflightMu.Lock()
	_, ok := b.inflight[42]
	b.inflightMu.Unlock()
	if !ok {
		t.Fatal("expected user 42 in inflight map")
	}
}

func TestStartInflight_CancelsPrevious(t *testing.T) {
	b := newTestBot()
	ctx1, cleanup1 := b.startInflight(42)
	defer cleanup1()

	_, cleanup2 := b.startInflight(42)
	defer cleanup2()

	if ctx1.Err() == nil {
		t.Fatal("first context should have been cancelled when second was started")
	}
}

func TestCancelInflight_ReturnsTrueWhenActive(t *testing.T) {
	b := newTestBot()
	ctx, cleanup := b.startInflight(42)
	defer cleanup()

	cancelled := b.cancelInflight(42)
	if !cancelled {
		t.Fatal("expected cancelInflight to return true")
	}
	if ctx.Err() == nil {
		t.Fatal("context should be cancelled")
	}
}

func TestCancelInflight_ReturnsFalseWhenNone(t *testing.T) {
	b := newTestBot()
	if b.cancelInflight(42) {
		t.Fatal("expected cancelInflight to return false when no request in flight")
	}
}

func TestCleanup_RemovesEntry(t *testing.T) {
	b := newTestBot()
	_, cleanup := b.startInflight(42)
	cleanup()

	b.inflightMu.Lock()
	_, ok := b.inflight[42]
	b.inflightMu.Unlock()
	if ok {
		t.Fatal("cleanup should have removed the entry")
	}
}

func TestCleanup_DoesNotRemoveNewerEntry(t *testing.T) {
	b := newTestBot()
	_, cleanup1 := b.startInflight(42)

	// Start a second request (cancels first, replaces entry).
	_, cleanup2 := b.startInflight(42)
	defer cleanup2()

	// Cleanup from first request should NOT remove the second's entry.
	cleanup1()

	b.inflightMu.Lock()
	_, ok := b.inflight[42]
	b.inflightMu.Unlock()
	if !ok {
		t.Fatal("cleanup of old request should not remove newer request's entry")
	}
}

func TestDispatchCancellable_NormalCompletion(t *testing.T) {
	b := newTestBot()
	done := make(chan string, 1)

	// Override sendMarkdown for testing by using dispatchCancellable directly.
	// We test the inflight lifecycle instead since sendMarkdown needs the API.
	ctx, cleanup := b.startInflight(99)

	go func() {
		defer cleanup()
		// Simulate work.
		select {
		case <-ctx.Done():
			done <- "cancelled"
		case <-time.After(10 * time.Millisecond):
			done <- "completed"
		}
	}()

	result := <-done
	if result != "completed" {
		t.Fatalf("expected completed, got %s", result)
	}

	// After completion and cleanup, entry should be gone.
	time.Sleep(5 * time.Millisecond) // let cleanup run
	b.inflightMu.Lock()
	_, ok := b.inflight[99]
	b.inflightMu.Unlock()
	if ok {
		t.Fatal("entry should be cleaned up after completion")
	}
}

func TestDispatchCancellable_Cancellation(t *testing.T) {
	b := newTestBot()
	done := make(chan string, 1)

	ctx, cleanup := b.startInflight(99)

	go func() {
		defer cleanup()
		select {
		case <-ctx.Done():
			done <- "cancelled"
		case <-time.After(5 * time.Second):
			done <- "completed"
		}
	}()

	// Cancel via /stop equivalent.
	b.cancelInflight(99)

	result := <-done
	if result != "cancelled" {
		t.Fatalf("expected cancelled, got %s", result)
	}
}

func TestConcurrentAccess(t *testing.T) {
	b := newTestBot()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()
			_, cleanup := b.startInflight(userID)
			time.Sleep(time.Millisecond)
			b.cancelInflight(userID)
			cleanup()
		}(int64(i % 5))
	}
	wg.Wait()
}
