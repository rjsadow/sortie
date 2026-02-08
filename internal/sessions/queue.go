package sessions

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	// DefaultMaxQueueSize is the maximum number of pending session requests.
	DefaultMaxQueueSize = 50

	// DefaultQueueTimeout is how long a request waits in the queue before being rejected.
	DefaultQueueTimeout = 30 * time.Second

	// DefaultQueuePollInterval is how often the queue checks for available capacity.
	DefaultQueuePollInterval = 500 * time.Millisecond
)

// QueueFullError is returned when the session queue is at capacity.
type QueueFullError struct {
	QueueSize    int
	MaxQueueSize int
}

func (e *QueueFullError) Error() string {
	return fmt.Sprintf("session queue full (%d/%d)", e.QueueSize, e.MaxQueueSize)
}

// QueueTimeoutError is returned when a queued request exceeds its deadline.
type QueueTimeoutError struct {
	WaitDuration time.Duration
}

func (e *QueueTimeoutError) Error() string {
	return fmt.Sprintf("session queue timeout after %v", e.WaitDuration)
}

// QueueConfig holds configuration for the session queue.
type QueueConfig struct {
	MaxSize      int           // Maximum queued requests (0 = disable queueing)
	Timeout      time.Duration // Per-request wait timeout
	PollInterval time.Duration // Capacity check interval
}

// queueEntry represents a pending session creation request.
type queueEntry struct {
	ready chan struct{} // closed when the entry may proceed
	err   error        // set if the entry is rejected
}

// SessionQueue manages pending session requests when capacity is full.
// When the global session limit is reached, new requests are queued (FIFO)
// instead of being immediately rejected. Queued requests are released as
// capacity becomes available (sessions end, expire, or fail).
type SessionQueue struct {
	mu       sync.Mutex
	entries  []*queueEntry
	config   QueueConfig
	checkCap func() bool // returns true when capacity is available
	stopCh   chan struct{}
}

// NewSessionQueue creates a queue with the given config and capacity checker.
// The capacityCheck function should return true when a new session can be created.
func NewSessionQueue(cfg QueueConfig, capacityCheck func() bool) *SessionQueue {
	if cfg.MaxSize == 0 {
		cfg.MaxSize = DefaultMaxQueueSize
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultQueueTimeout
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultQueuePollInterval
	}

	q := &SessionQueue{
		config:   cfg,
		checkCap: capacityCheck,
		stopCh:   make(chan struct{}),
	}

	go q.processLoop()
	return q
}

// Enqueue adds a request to the queue and blocks until capacity is available
// or the context/timeout expires. Returns nil when the caller may proceed,
// or an error if the queue is full or the wait timed out.
func (q *SessionQueue) Enqueue(ctx context.Context) error {
	q.mu.Lock()

	// Check if capacity is already available (fast path)
	if q.checkCap() {
		q.mu.Unlock()
		return nil
	}

	// Check queue size limit
	if len(q.entries) >= q.config.MaxSize {
		size := len(q.entries)
		q.mu.Unlock()
		return &QueueFullError{QueueSize: size, MaxQueueSize: q.config.MaxSize}
	}

	entry := &queueEntry{
		ready: make(chan struct{}),
	}
	q.entries = append(q.entries, entry)
	position := len(q.entries)
	q.mu.Unlock()

	log.Printf("Session queued at position %d (queue size: %d)", position, position)

	// Wait for the entry to be released or timeout
	timeout := time.NewTimer(q.config.Timeout)
	defer timeout.Stop()

	select {
	case <-entry.ready:
		return entry.err
	case <-timeout.C:
		q.remove(entry)
		return &QueueTimeoutError{WaitDuration: q.config.Timeout}
	case <-ctx.Done():
		q.remove(entry)
		return ctx.Err()
	case <-q.stopCh:
		return fmt.Errorf("session queue shutting down")
	}
}

// Len returns the current queue depth.
func (q *SessionQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}

// NotifyCapacity wakes the processing loop to re-check capacity.
// Call this when a session ends (stopped, expired, terminated, failed).
func (q *SessionQueue) NotifyCapacity() {
	// The processLoop polls periodically; this is a hint to check sooner.
	// We don't need a signal channel because the poll interval is short.
}

// Stop shuts down the queue processing loop.
func (q *SessionQueue) Stop() {
	close(q.stopCh)
}

// processLoop periodically checks for capacity and releases queued entries.
func (q *SessionQueue) processLoop() {
	ticker := time.NewTicker(q.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			q.tryRelease()
		case <-q.stopCh:
			q.drainOnShutdown()
			return
		}
	}
}

// tryRelease releases the front-of-queue entry if capacity is available.
func (q *SessionQueue) tryRelease() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.entries) == 0 {
		return
	}

	if q.checkCap() {
		entry := q.entries[0]
		q.entries = q.entries[1:]
		close(entry.ready)
		log.Printf("Session dequeued (remaining: %d)", len(q.entries))
	}
}

// remove removes a specific entry from the queue (for timeout/cancellation).
func (q *SessionQueue) remove(target *queueEntry) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, e := range q.entries {
		if e == target {
			q.entries = append(q.entries[:i], q.entries[i+1:]...)
			return
		}
	}
}

// drainOnShutdown rejects all queued entries during shutdown.
func (q *SessionQueue) drainOnShutdown() {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, entry := range q.entries {
		entry.err = fmt.Errorf("session queue shutting down")
		close(entry.ready)
	}
	q.entries = nil
}
