package lightweightapi

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

const DefaultHeartbeatBatchInterval = 30 * time.Second

// BatchedHeartbeatScheduler coalesces runtime heartbeats into one UPDATE per
// interval. Runtime ownership is validated before Schedule is called.
type BatchedHeartbeatScheduler struct {
	queries  *lwdb.Queries
	interval time.Duration

	mu      sync.Mutex
	pending map[pgtype.UUID]struct{}
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func NewBatchedHeartbeatScheduler(queries *lwdb.Queries, interval time.Duration) *BatchedHeartbeatScheduler {
	if interval <= 0 {
		interval = DefaultHeartbeatBatchInterval
	}
	return &BatchedHeartbeatScheduler{
		queries: queries, interval: interval, pending: make(map[pgtype.UUID]struct{}),
		stop: make(chan struct{}), done: make(chan struct{}),
	}
}

func (s *BatchedHeartbeatScheduler) Schedule(runtimeID pgtype.UUID) {
	if !runtimeID.Valid {
		return
	}
	s.mu.Lock()
	s.pending[runtimeID] = struct{}{}
	s.mu.Unlock()
}

func (s *BatchedHeartbeatScheduler) Run(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.flushWithTimeout()
			return
		case <-s.stop:
			s.flushWithTimeout()
			return
		case <-ticker.C:
			s.flush(ctx)
		}
	}
}

func (s *BatchedHeartbeatScheduler) Stop() {
	s.once.Do(func() { close(s.stop) })
	<-s.done
}

func (s *BatchedHeartbeatScheduler) FlushNow(ctx context.Context) { s.flush(ctx) }

func (s *BatchedHeartbeatScheduler) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}

func (s *BatchedHeartbeatScheduler) flushWithTimeout() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.flush(ctx)
}

func (s *BatchedHeartbeatScheduler) flush(ctx context.Context) {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return
	}
	ids := make([]pgtype.UUID, 0, len(s.pending))
	for id := range s.pending {
		ids = append(ids, id)
	}
	s.pending = make(map[pgtype.UUID]struct{})
	s.mu.Unlock()
	if _, err := s.queries.TouchAgentRuntimesBatch(ctx, ids); err != nil {
		slog.Warn("heartbeat batch flush failed", "scheduled", len(ids), "error", err)
	}
}
