package mcpgateway

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestKeyedGateBoundsConcurrencyAndCleansCancelledWaiters(t *testing.T) {
	gate := newKeyedGate(2)
	first, err := gate.acquire(context.Background(), "task-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := gate.acquire(context.Background(), "task-a")
	if err != nil {
		t.Fatal(err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := gate.acquire(waitCtx, "task-a"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("third acquisition error = %v, want deadline exceeded", err)
	}

	other, err := gate.acquire(context.Background(), "task-b")
	if err != nil {
		t.Fatalf("independent key was blocked: %v", err)
	}
	other()
	first()
	first()
	second()

	gate.mu.Lock()
	remaining := len(gate.keys)
	gate.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("gate retained %d key entries after release and cancellation", remaining)
	}
}
