package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/kms9/dars/internal/lightweightapi"
)

const lightweightSweepInterval = 30 * time.Second

func runLightweightTaskSweeper(ctx context.Context, core *lightweightapi.Handler) {
	ticker := time.NewTicker(lightweightSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := core.SweepTaskCore(ctx)
			if err != nil {
				slog.Warn("lightweight task sweep failed", "error", err)
				continue
			}
			if result.StaleRuntimes+result.FailedTasks+result.RequeuedLeases+result.PromotedDeferred+result.FinalizedCancels > 0 {
				slog.Info("lightweight task sweep completed", "result", result)
			}
		}
	}
}
