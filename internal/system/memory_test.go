package system

import (
	"context"
	"testing"
	"time"
)

func TestMemoryManager_AcquireRelease(t *testing.T) {
	m := NewMemoryManager(1) // 1MB limit
	limit := m.totalLimit

	ctx := context.Background()

	// Acquire all memory
	err := m.Acquire(ctx, limit)
	if err != nil {
		t.Fatalf("Failed to acquire memory: %v", err)
	}

	if m.used != limit {
		t.Errorf("Expected used %d, got %d", limit, m.used)
	}

	// Try to acquire more in a separate goroutine (should block)
	errChan := make(chan error, 1)
	go func() {
		errChan <- m.Acquire(ctx, 1)
	}()

	select {
	case err := <-errChan:
		t.Fatalf("Acquire should have blocked, but returned: %v", err)
	case <-time.After(100 * time.Millisecond):
		// Success: it's blocking
	}

	// Release memory
	m.Release(limit)
	if m.used != 0 {
		t.Errorf("Expected used 0, got %d", m.used)
	}

	// Now the blocked goroutine should succeed
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Acquire failed after release: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Errorf("Acquire did not unblock after release")
	}
}

func TestMemoryManager_ContextCancel(t *testing.T) {
	m := NewMemoryManager(1)
	limit := m.totalLimit

	_ = m.Acquire(context.Background(), limit)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := m.Acquire(ctx, 1)
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}
}
