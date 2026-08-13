package lightweightapi

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

const (
	lightweightRuntimeStaleAfter = 150 * time.Second
	lightweightQueuedTTL         = 2 * time.Hour
	lightweightCancelGrace       = 60 * time.Second
	lightweightSweepBatch        = int32(500)
)

type TaskSweepResult struct {
	StaleRuntimes        int `json:"stale_runtimes"`
	FailedTasks          int `json:"failed_tasks"`
	RetriedTasks         int `json:"retried_tasks"`
	ExpiredQueued        int `json:"expired_queued"`
	RequeuedLeases       int `json:"requeued_leases"`
	PromotedDeferred     int `json:"promoted_deferred"`
	FinalizedCancels     int `json:"finalized_cancels"`
	ExpiredVerifications int `json:"expired_verifications"`
	ExpiredDaemonTokens  int `json:"expired_daemon_tokens"`
	ExpiredTaskTokens    int `json:"expired_task_tokens"`
}

type sweptFailure struct {
	task    lwdb.AgentTaskQueue
	retry   *lwdb.AgentTaskQueue
	message *lwdb.ChatMessage
}

func (h *Handler) settleSweptFailure(ctx context.Context, q *lwdb.Queries, task lwdb.AgentTaskQueue, failureReason, errorMessage string) (sweptFailure, error) {
	result := sweptFailure{task: task}
	if _, err := q.DeleteTaskTokensByTask(ctx, task.ID); err != nil {
		return sweptFailure{}, err
	}
	retry, err := h.createRetryTask(ctx, q, task, failureReason)
	if err != nil {
		return sweptFailure{}, err
	}
	result.retry = retry
	if retry == nil {
		result.message, err = h.writeFailureProjection(ctx, q, task, taskFailRequest{Error: errorMessage, FailureReason: failureReason})
		if err != nil {
			return sweptFailure{}, err
		}
	}
	if err := updateChatResume(ctx, q, task); err != nil {
		return sweptFailure{}, err
	}
	return result, nil
}

func (h *Handler) announceSweptFailure(result sweptFailure) {
	if result.message != nil {
		h.publish(protocol.EventChatMessage, uuidString(result.task.WorkspaceID), "system", "", protocol.ChatMessagePayload{
			ChatSessionID: uuidString(result.task.ChatSessionID), MessageID: uuidString(result.message.ID), Role: result.message.Role,
			Content: result.message.Content, TaskID: uuidString(result.task.ID), CreatedAt: timeString(result.message.CreatedAt),
		})
	}
	h.publish(protocol.EventTaskFailed, uuidString(result.task.WorkspaceID), "system", "", taskLifecycleDTO(result.task))
	if result.retry != nil && result.retry.Status == "queued" {
		h.announceQueuedTask(*result.retry)
	}
}

func (h *Handler) sweepStaleRuntimes(ctx context.Context, result *TaskSweepResult) error {
	staleBefore := pgtype.Timestamptz{Time: h.now().Add(-lightweightRuntimeStaleAfter), Valid: true}
	runtimes, err := h.q.ListStaleAgentRuntimes(ctx, lwdb.ListStaleAgentRuntimesParams{StaleBefore: staleBefore, BatchLimit: lightweightSweepBatch})
	if err != nil {
		return err
	}
	for _, candidate := range runtimes {
		tx, err := h.pool.Begin(ctx)
		if err != nil {
			return err
		}
		qtx := h.q.WithTx(tx)
		if _, err := qtx.MarkAgentRuntimeOffline(ctx, lwdb.MarkAgentRuntimeOfflineParams{
			ID: candidate.ID, WorkspaceID: candidate.WorkspaceID, StaleBefore: staleBefore,
		}); err != nil {
			_ = tx.Rollback(ctx)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return err
		}
		tasks, err := qtx.FailActiveTasksForRuntime(ctx, lwdb.FailActiveTasksForRuntimeParams{
			Error:         pgtype.Text{String: "runtime heartbeat expired", Valid: true},
			FailureReason: pgtype.Text{String: "runtime_offline", Valid: true},
			WorkspaceID:   candidate.WorkspaceID, RuntimeID: candidate.ID,
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		settled := make([]sweptFailure, 0, len(tasks))
		for _, task := range tasks {
			failure, err := h.settleSweptFailure(ctx, qtx, task, "runtime_offline", "runtime heartbeat expired")
			if err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			settled = append(settled, failure)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		result.StaleRuntimes++
		result.FailedTasks += len(settled)
		for _, failure := range settled {
			if failure.retry != nil {
				result.RetriedTasks++
			}
			h.announceSweptFailure(failure)
		}
	}
	return nil
}

func (h *Handler) sweepExpiredQueued(ctx context.Context, result *TaskSweepResult) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := h.q.WithTx(tx)
	tasks, err := qtx.ExpireStaleQueuedTasks(ctx, lwdb.ExpireStaleQueuedTasksParams{
		ExpireBefore: pgtype.Timestamptz{Time: h.now().Add(-lightweightQueuedTTL), Valid: true}, BatchLimit: lightweightSweepBatch,
	})
	if err != nil {
		return err
	}
	settled := make([]sweptFailure, 0, len(tasks))
	for _, task := range tasks {
		failure, err := h.settleSweptFailure(ctx, qtx, task, "queue_expired", "task expired before it could be claimed")
		if err != nil {
			return err
		}
		settled = append(settled, failure)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	result.ExpiredQueued += len(settled)
	result.FailedTasks += len(settled)
	for _, failure := range settled {
		h.announceSweptFailure(failure)
	}
	return nil
}

func (h *Handler) sweepDeferredCancels(ctx context.Context, result *TaskSweepResult) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := h.q.WithTx(tx)
	tasks, err := qtx.ListDeferredCancelledTasks(ctx, lwdb.ListDeferredCancelledTasksParams{
		GraceSeconds: lightweightCancelGrace.Seconds(), BatchLimit: lightweightSweepBatch,
	})
	if err != nil {
		return err
	}
	type finalizedCancel struct {
		task      lwdb.AgentTaskQueue
		initiator string
	}
	finalized := make([]finalizedCancel, 0, len(tasks))
	for _, task := range tasks {
		if _, err := createCancelledChatDraft(ctx, qtx, task); err != nil {
			return err
		}
		updated, err := qtx.FinalizeCancelledTask(ctx, lwdb.FinalizeCancelledTaskParams{ID: task.ID, WorkspaceID: task.WorkspaceID})
		if err != nil {
			return err
		}
		if _, err := qtx.DeleteTaskTokensByTask(ctx, task.ID); err != nil {
			return err
		}
		finalized = append(finalized, finalizedCancel{task: updated, initiator: cancelledChatInitiator(ctx, qtx, updated)})
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	result.FinalizedCancels += len(finalized)
	for _, item := range finalized {
		h.publishCancelledChatFinalized(item.task, item.initiator)
	}
	return nil
}

// SweepTaskCore performs one bounded recovery tick against only the
// Lightweight baseline. Each state-changing group commits before its events
// are published; retry and cancellation projections share the same transaction
// as the authoritative task transition.
func (h *Handler) SweepTaskCore(ctx context.Context) (TaskSweepResult, error) {
	result := TaskSweepResult{}
	if err := h.sweepStaleRuntimes(ctx, &result); err != nil {
		return result, err
	}
	if err := h.sweepExpiredQueued(ctx, &result); err != nil {
		return result, err
	}
	requeued, err := h.q.RequeueExpiredPrepareLeases(ctx, lightweightSweepBatch)
	if err != nil {
		return result, err
	}
	result.RequeuedLeases = len(requeued)
	for _, task := range requeued {
		h.announceQueuedTask(task)
	}
	promoted, err := h.q.PromoteDeferredTasks(ctx, lightweightSweepBatch)
	if err != nil {
		return result, err
	}
	result.PromotedDeferred = len(promoted)
	for _, task := range promoted {
		h.announceQueuedTask(task)
	}
	if err := h.sweepDeferredCancels(ctx, &result); err != nil {
		return result, err
	}
	if count, err := h.q.DeleteExpiredVerificationCodes(ctx); err != nil {
		return result, err
	} else {
		result.ExpiredVerifications = int(count)
	}
	if count, err := h.q.DeleteExpiredDaemonTokens(ctx); err != nil {
		return result, err
	} else {
		result.ExpiredDaemonTokens = int(count)
	}
	if count, err := h.q.DeleteExpiredTaskTokens(ctx); err != nil {
		return result, err
	} else {
		result.ExpiredTaskTokens = int(count)
	}
	return result, nil
}
