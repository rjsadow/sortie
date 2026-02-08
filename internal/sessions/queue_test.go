package sessions

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionQueue_FastPath(t *testing.T) {
	// When capacity is available, Enqueue returns immediately.
	q := NewSessionQueue(QueueConfig{
		MaxSize:      10,
		Timeout:      time.Second,
		PollInterval: 50 * time.Millisecond,
	}, func() bool { return true })
	defer q.Stop()

	err := q.Enqueue(context.Background())
	if err != nil {
		t.Fatalf("Enqueue() with available capacity should succeed, got %v", err)
	}
	if q.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (fast path should not enqueue)", q.Len())
	}
}

func TestSessionQueue_WaitAndRelease(t *testing.T) {
	// Capacity starts unavailable, then becomes available.
	var hasCapacity atomic.Bool
	hasCapacity.Store(false)

	q := NewSessionQueue(QueueConfig{
		MaxSize:      10,
		Timeout:      2 * time.Second,
		PollInterval: 50 * time.Millisecond,
	}, func() bool { return hasCapacity.Load() })
	defer q.Stop()

	done := make(chan error, 1)
	go func() {
		done <- q.Enqueue(context.Background())
	}()

	// Verify request is queued
	time.Sleep(100 * time.Millisecond)
	if q.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", q.Len())
	}

	// Release capacity
	hasCapacity.Store(true)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Enqueue() should succeed after capacity released, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Enqueue() did not return after capacity was released")
	}
}

func TestSessionQueue_Timeout(t *testing.T) {
	q := NewSessionQueue(QueueConfig{
		MaxSize:      10,
		Timeout:      200 * time.Millisecond,
		PollInterval: 50 * time.Millisecond,
	}, func() bool { return false })
	defer q.Stop()

	start := time.Now()
	err := q.Enqueue(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Enqueue() should return error on timeout")
	}
	if _, ok := err.(*QueueTimeoutError); !ok {
		t.Errorf("expected QueueTimeoutError, got %T: %v", err, err)
	}
	if elapsed < 150*time.Millisecond {
		t.Errorf("timeout should wait ~200ms, returned after %v", elapsed)
	}
}

func TestSessionQueue_ContextCancellation(t *testing.T) {
	q := NewSessionQueue(QueueConfig{
		MaxSize:      10,
		Timeout:      5 * time.Second,
		PollInterval: 50 * time.Millisecond,
	}, func() bool { return false })
	defer q.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := q.Enqueue(ctx)
	if err == nil {
		t.Fatal("Enqueue() should return error on context cancellation")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestSessionQueue_Full(t *testing.T) {
	q := NewSessionQueue(QueueConfig{
		MaxSize:      2,
		Timeout:      5 * time.Second,
		PollInterval: 50 * time.Millisecond,
	}, func() bool { return false })
	defer q.Stop()

	// Fill the queue with 2 background requests
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			q.Enqueue(ctx) //nolint:errcheck
		}()
	}

	// Wait for queue to fill
	time.Sleep(100 * time.Millisecond)

	// Third request should get QueueFullError
	err := q.Enqueue(context.Background())
	if err == nil {
		t.Fatal("Enqueue() should return error when queue is full")
	}
	if _, ok := err.(*QueueFullError); !ok {
		t.Errorf("expected QueueFullError, got %T: %v", err, err)
	}

	wg.Wait()
}

func TestSessionQueue_FIFO(t *testing.T) {
	// Verify requests are released in FIFO order.
	var hasCapacity atomic.Bool
	hasCapacity.Store(false)

	q := NewSessionQueue(QueueConfig{
		MaxSize:      10,
		Timeout:      2 * time.Second,
		PollInterval: 50 * time.Millisecond,
	}, func() bool {
		// Only allow one at a time
		return hasCapacity.CompareAndSwap(true, false)
	})
	defer q.Stop()

	order := make([]int, 0, 3)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			err := q.Enqueue(context.Background())
			if err == nil {
				mu.Lock()
				order = append(order, idx)
				mu.Unlock()
			}
		}()
		// Stagger enqueue to ensure deterministic order
		time.Sleep(50 * time.Millisecond)
	}

	// Release one at a time
	for i := 0; i < 3; i++ {
		hasCapacity.Store(true)
		time.Sleep(150 * time.Millisecond)
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	if len(order) != 3 {
		t.Fatalf("expected 3 completions, got %d", len(order))
	}
	for i, idx := range order {
		if idx != i {
			t.Errorf("order[%d] = %d, want %d (FIFO violation)", i, idx, i)
		}
	}
}

func TestSessionQueue_Shutdown(t *testing.T) {
	q := NewSessionQueue(QueueConfig{
		MaxSize:      10,
		Timeout:      5 * time.Second,
		PollInterval: 50 * time.Millisecond,
	}, func() bool { return false })

	done := make(chan error, 1)
	go func() {
		done <- q.Enqueue(context.Background())
	}()

	time.Sleep(100 * time.Millisecond)
	q.Stop()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Enqueue() should return error on shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("Enqueue() did not return after shutdown")
	}
}

func TestSessionQueue_LenAccuracy(t *testing.T) {
	var hasCapacity atomic.Bool
	hasCapacity.Store(false)

	q := NewSessionQueue(QueueConfig{
		MaxSize:      10,
		Timeout:      2 * time.Second,
		PollInterval: 50 * time.Millisecond,
	}, func() bool { return hasCapacity.Load() })
	defer q.Stop()

	if q.Len() != 0 {
		t.Errorf("empty queue Len() = %d, want 0", q.Len())
	}

	// Add 3 entries
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			q.Enqueue(ctx) //nolint:errcheck
		}()
	}

	time.Sleep(100 * time.Millisecond)
	if q.Len() != 3 {
		t.Errorf("Len() = %d, want 3", q.Len())
	}

	wg.Wait()
}
