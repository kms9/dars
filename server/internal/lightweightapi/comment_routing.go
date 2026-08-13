package lightweightapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kms9/dars/internal/util"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

type resolvedCommentTarget struct {
	targetType string
	targetID   pgtype.UUID
	assignee   issueAssigneeResolution
}

func commentRoutingUser(ctx context.Context, queries *lwdb.Queries, principal Principal, sourceTask *lwdb.AgentTaskQueue) (pgtype.UUID, error) {
	if sourceTask != nil {
		for _, candidate := range []pgtype.UUID{sourceTask.OriginatorUserID, sourceTask.AccountableUserID, sourceTask.InitiatorUserID} {
			if candidate.Valid {
				return candidate, nil
			}
		}
		agent, err := queries.GetAgent(ctx, lwdb.GetAgentParams{ID: sourceTask.AgentID, WorkspaceID: sourceTask.WorkspaceID})
		if err == nil {
			return agent.OwnerID, nil
		}
	}
	return parseUUID(principal.UserID)
}

func addResolvedCommentTarget(targets map[string]resolvedCommentTarget, target resolvedCommentTarget) {
	key := uuidString(target.assignee.agentID)
	existing, ok := targets[key]
	if !ok || (!existing.assignee.squadID.Valid && target.assignee.squadID.Valid) {
		targets[key] = target
	}
}

// resolveCommentTargets 根据评论内容决定要入队的执行目标。
// 1) 解析 mention://agent|squad；2) 跳过 sourceTask 自提及（防 Leader 循环）；
// 3) 小队成员 mention 时标记 is_leader_task=false；
// 4) 人类无 mention 的跟帖：已执行过的 Run 回指派给当前 assignee；
// 5) 成员任务结果（!IsLeaderTask）：即使无 @ 也回传本 Squad Leader。
func (h *Handler) resolveCommentTargets(
	ctx context.Context,
	queries *lwdb.Queries,
	issue lwdb.Issue,
	comment lwdb.Comment,
	principal Principal,
	sourceTask *lwdb.AgentTaskQueue,
) ([]resolvedCommentTarget, pgtype.UUID, error) {
	routingUser, err := commentRoutingUser(ctx, queries, principal, sourceTask)
	if err != nil {
		return nil, pgtype.UUID{}, issueResolutionFailure(http.StatusUnauthorized, "unauthenticated")
	}
	targets := make(map[string]resolvedCommentTarget)
	for _, mention := range util.ParseMentions(comment.Content) {
		if mention.Type != "agent" && mention.Type != "squad" {
			continue
		}
		targetID, err := parseUUID(mention.ID)
		if err != nil {
			return nil, pgtype.UUID{}, issueResolutionFailure(http.StatusBadRequest, "invalid_mention")
		}
		resolved, err := resolveIssueAssignee(ctx, queries, issue.WorkspaceID, routingUser, mention.Type, targetID)
		if err != nil {
			var resolution *issueResolutionError
			if errors.As(err, &resolution) && resolution.code == "not_found" {
				return nil, pgtype.UUID{}, issueResolutionFailure(http.StatusBadRequest, "mention_target_not_found")
			}
			return nil, pgtype.UUID{}, err
		}
		if mention.Type == "agent" && sourceTask != nil && sourceTask.SquadID.Valid {
			members, listErr := queries.ListSquadMembers(ctx, sourceTask.SquadID)
			if listErr != nil {
				return nil, pgtype.UUID{}, listErr
			}
			for _, member := range members {
				if member.AgentID == resolved.agentID && member.Role != "leader" {
					resolved.squadID = sourceTask.SquadID
					resolved.leader = false
					break
				}
			}
		}
		if sourceTask != nil && sourceTask.AgentID == resolved.agentID {
			// A task may describe its own work, but it must never enqueue itself.
			// This includes a Squad Leader mentioning its own squad.
			continue
		}
		addResolvedCommentTarget(targets, resolvedCommentTarget{targetType: mention.Type, targetID: targetID, assignee: resolved})
	}

	if sourceTask == nil {
		// A normal human follow-up belongs to the already-started Run even when
		// it has no explicit mention. Backlog comments remain notes until the Run
		// first enters todo.
		if issue.FirstExecutedAt.Valid {
			resolved, err := resolveIssueAssignee(ctx, queries, issue.WorkspaceID, routingUser, issue.AssigneeType, issue.AssigneeID)
			if err != nil {
				return nil, pgtype.UUID{}, err
			}
			addResolvedCommentTarget(targets, resolvedCommentTarget{targetType: issue.AssigneeType, targetID: issue.AssigneeID, assignee: resolved})
		}
	} else if sourceTask.SquadID.Valid && !sourceTask.IsLeaderTask {
		// A Member result always returns to its own Squad Leader, even when the
		// result contains no explicit mention.
		resolved, err := resolveIssueAssignee(ctx, queries, issue.WorkspaceID, routingUser, "squad", sourceTask.SquadID)
		if err != nil {
			return nil, pgtype.UUID{}, err
		}
		if sourceTask.AgentID != resolved.agentID {
			addResolvedCommentTarget(targets, resolvedCommentTarget{targetType: "squad", targetID: sourceTask.SquadID, assignee: resolved})
		}
	}

	result := make([]resolvedCommentTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, target)
	}
	return result, routingUser, nil
}

func commentTriggerSummary(content string) pgtype.Text {
	content = strings.TrimSpace(content)
	if content == "" {
		return pgtype.Text{}
	}
	const maxRunes = 500
	runes := []rune(content)
	if len(runes) > maxRunes {
		content = string(runes[:maxRunes])
	}
	return pgtype.Text{String: content, Valid: true}
}

func commentTaskAttribution(principal Principal, sourceTask *lwdb.AgentTaskQueue, routingUser pgtype.UUID) (pgtype.UUID, pgtype.UUID, pgtype.UUID, pgtype.Text, pgtype.UUID) {
	initiator, originator, accountable := routingUser, routingUser, routingUser
	source := pgtype.Text{String: "direct_human", Valid: true}
	var delegatedFrom pgtype.UUID
	if sourceTask != nil {
		if sourceTask.InitiatorUserID.Valid {
			initiator = sourceTask.InitiatorUserID
		}
		if sourceTask.OriginatorUserID.Valid {
			originator = sourceTask.OriginatorUserID
		}
		if sourceTask.AccountableUserID.Valid {
			accountable = sourceTask.AccountableUserID
		}
		source = pgtype.Text{String: "delegation", Valid: true}
		delegatedFrom = sourceTask.ID
	}
	return initiator, originator, accountable, source, delegatedFrom
}

// enqueueCommentTarget 为单个路由目标入队或合并评论。
// 若该 Agent 在本 Issue 上已有活跃 task：把评论追加到 coalesced（不新建队列）；
// 否则 CreateAgentTask（queued），带上 squad_id / is_leader_task / delegated_from。
func (h *Handler) enqueueCommentTarget(
	ctx context.Context,
	queries *lwdb.Queries,
	issue lwdb.Issue,
	prefix string,
	comment lwdb.Comment,
	principal Principal,
	sourceTask *lwdb.AgentTaskQueue,
	routingUser pgtype.UUID,
	target resolvedCommentTarget,
) (*lwdb.AgentTaskQueue, error) {
	active, err := queries.LockActiveTaskForIssueAgent(ctx, lwdb.LockActiveTaskForIssueAgentParams{
		WorkspaceID: issue.WorkspaceID, IssueID: issue.ID, AgentID: target.assignee.agentID,
	})
	if err == nil {
		if _, err := queries.AppendPlannedCommentToTask(ctx, lwdb.AppendPlannedCommentToTaskParams{
			CommentID: comment.ID, ID: active.ID, WorkspaceID: issue.WorkspaceID,
		}); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err := h.requireGatewayProviderForNewTask(ctx, queries, issue.WorkspaceID, target.assignee.agentID, target.assignee.runtimeID); err != nil {
		return nil, err
	}
	initiator, originator, accountable, originatorSource, delegatedFrom := commentTaskAttribution(principal, sourceTask, routingUser)
	task, err := queries.CreateAgentTask(ctx, lwdb.CreateAgentTaskParams{
		WorkspaceID: issue.WorkspaceID, AgentID: target.assignee.agentID, RuntimeID: target.assignee.runtimeID,
		IssueID: issue.ID, SquadID: target.assignee.squadID, IsLeaderTask: target.assignee.leader,
		Status: "queued", Priority: 0, Attempt: 1, MaxAttempts: 2,
		Context: issueTaskContext(issue, prefix), RuntimeMcpOverlay: []byte(`{}`),
		InitiatorUserID: initiator, OriginatorUserID: originator, AccountableUserID: accountable,
		OriginatorSource: originatorSource, TriggerCommentID: comment.ID, CoalescedCommentIds: []pgtype.UUID{},
		DelegatedFromTaskID: delegatedFrom, TriggerEvidenceKind: pgtype.Text{String: "comment", Valid: true},
		TriggerEvidenceRefID: comment.ID, TriggerSummary: commentTriggerSummary(comment.Content),
	})
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// routeCreatedComment 评论创建后的入队编排入口。
// resolveCommentTargets → 对每个目标 enqueueCommentTarget，返回新建的 queued tasks
// （调用方再 announceQueuedTask）。Leader 派活与成员 complete 投影评论都走此路径。
func (h *Handler) routeCreatedComment(
	ctx context.Context,
	queries *lwdb.Queries,
	issue lwdb.Issue,
	comment lwdb.Comment,
	principal Principal,
	sourceTask *lwdb.AgentTaskQueue,
) ([]lwdb.AgentTaskQueue, error) {
	targets, routingUser, err := h.resolveCommentTargets(ctx, queries, issue, comment, principal, sourceTask)
	if err != nil {
		return nil, err
	}
	workspace, err := queries.GetWorkspace(ctx, issue.WorkspaceID)
	if err != nil {
		return nil, err
	}
	queued := make([]lwdb.AgentTaskQueue, 0, len(targets))
	for _, target := range targets {
		task, err := h.enqueueCommentTarget(ctx, queries, issue, workspace.IssuePrefix, comment, principal, sourceTask, routingUser, target)
		if err != nil {
			return nil, err
		}
		if task != nil {
			queued = append(queued, *task)
		}
	}
	return queued, nil
}

func uuidSet(values []pgtype.UUID) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value.Valid {
			result[uuidString(value)] = struct{}{}
		}
	}
	return result
}

func undeliveredCommentIDs(task lwdb.AgentTaskQueue) []pgtype.UUID {
	delivered := uuidSet(task.DeliveredCommentIds)
	planned := make([]pgtype.UUID, 0, len(task.CoalescedCommentIds)+1)
	if task.TriggerCommentID.Valid {
		planned = append(planned, task.TriggerCommentID)
	}
	planned = append(planned, task.CoalescedCommentIds...)
	seen := make(map[string]struct{}, len(planned))
	result := make([]pgtype.UUID, 0, len(planned))
	for _, id := range planned {
		key := uuidString(id)
		if !id.Valid {
			continue
		}
		if _, ok := delivered[key]; ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, id)
	}
	return result
}

// reconcileUndeliveredComments 在任务结束后补投递尚未交付的评论。
// 若同 Agent 仍有活跃 task：把未交付评论追加进去；否则为剩余评论创建一条 follow-up task，
// 避免 Leader 执行期间到达的成员结果丢失。
func (h *Handler) reconcileUndeliveredComments(ctx context.Context, queries *lwdb.Queries, issue lwdb.Issue, task lwdb.AgentTaskQueue) (*lwdb.AgentTaskQueue, error) {
	undelivered := undeliveredCommentIDs(task)
	if len(undelivered) == 0 {
		return nil, nil
	}
	active, err := queries.LockActiveTaskForIssueAgent(ctx, lwdb.LockActiveTaskForIssueAgentParams{
		WorkspaceID: issue.WorkspaceID, IssueID: issue.ID, AgentID: task.AgentID,
	})
	if err == nil {
		for _, commentID := range undelivered {
			if _, err := queries.AppendPlannedCommentToTask(ctx, lwdb.AppendPlannedCommentToTaskParams{
				CommentID: commentID, ID: active.ID, WorkspaceID: issue.WorkspaceID,
			}); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	workspace, err := queries.GetWorkspace(ctx, issue.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if task.ToolBundleID.Valid {
		if err := h.requireGatewayProviderForPinnedTask(ctx, queries, issue.WorkspaceID, task.RuntimeID); err != nil {
			return nil, err
		}
	}
	coalesced := append(make([]pgtype.UUID, 0, len(undelivered)-1), undelivered[1:]...)
	followup, err := queries.CreateAgentTask(ctx, lwdb.CreateAgentTaskParams{
		WorkspaceID: issue.WorkspaceID, AgentID: task.AgentID, RuntimeID: task.RuntimeID,
		IssueID: issue.ID, SquadID: task.SquadID, IsLeaderTask: task.IsLeaderTask,
		Status: "queued", Priority: task.Priority, Attempt: 1, MaxAttempts: task.MaxAttempts,
		ParentTaskID: task.ID, Context: issueTaskContext(issue, workspace.IssuePrefix), RuntimeMcpOverlay: task.RuntimeMcpOverlay,
		InitiatorUserID: task.InitiatorUserID, OriginatorUserID: task.OriginatorUserID,
		AccountableUserID: task.AccountableUserID, OriginatorSource: pgtype.Text{String: "comment_reconciliation", Valid: true},
		TriggerCommentID: undelivered[0], CoalescedCommentIds: coalesced, DelegatedFromTaskID: task.ID,
		TriggerEvidenceKind: pgtype.Text{String: "comment_reconciliation", Valid: true}, TriggerEvidenceRefID: undelivered[0],
		TriggerSummary: pgtype.Text{String: "Process comments that arrived after the previous task was claimed.", Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return &followup, nil
}
