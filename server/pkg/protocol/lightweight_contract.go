package protocol

import (
	"net/http"
	"net/url"
	"strings"
)

// LightweightDaemonRoute is one method+path template in the frozen daemon
// HTTP contract. Server registration and daemon client path construction use
// the constants below so a path cutover cannot drift independently.
type LightweightDaemonRoute struct {
	Method string
	Path   string
}

const (
	DaemonRouteRegister                 = "/api/daemon/register"
	DaemonRouteDeregister               = "/api/daemon/deregister"
	DaemonRouteHeartbeat                = "/api/daemon/heartbeat"
	DaemonRouteWebSocket                = "/api/daemon/ws"
	DaemonRouteWorkspaces               = "/api/daemon/workspaces"
	DaemonRouteWorkspaceRepos           = "/api/daemon/workspaces/{workspaceId}/repos"
	DaemonRouteWorkspaceRuntimeProfiles = "/api/daemon/workspaces/{workspaceId}/runtime-profiles"
	DaemonRouteTasksClaim               = "/api/daemon/tasks/claim"
	DaemonRouteTaskPrepareLease         = "/api/daemon/runtimes/{runtimeId}/tasks/{taskId}/prepare-lease"
	DaemonRouteTaskSkillBundlesResolve  = "/api/daemon/runtimes/{runtimeId}/tasks/{taskId}/skill-bundles/resolve"
	DaemonRouteRuntimePendingTasks      = "/api/daemon/runtimes/{runtimeId}/tasks/pending"
	DaemonRouteModelListResult          = "/api/daemon/runtimes/{runtimeId}/models/{requestId}/result"
	DaemonRouteLocalSkillListResult     = "/api/daemon/runtimes/{runtimeId}/local-skills/{requestId}/result"
	DaemonRouteLocalSkillImportResult   = "/api/daemon/runtimes/{runtimeId}/local-skills/import/{requestId}/result"
	DaemonRouteTaskStatus               = "/api/daemon/tasks/{taskId}/status"
	DaemonRouteTaskStart                = "/api/daemon/tasks/{taskId}/start"
	DaemonRouteTaskWaitLocalDirectory   = "/api/daemon/tasks/{taskId}/wait-local-directory"
	DaemonRouteTaskProgress             = "/api/daemon/tasks/{taskId}/progress"
	DaemonRouteTaskMessages             = "/api/daemon/tasks/{taskId}/messages"
	DaemonRouteTaskUsage                = "/api/daemon/tasks/{taskId}/usage"
	DaemonRouteTaskComplete             = "/api/daemon/tasks/{taskId}/complete"
	DaemonRouteTaskFail                 = "/api/daemon/tasks/{taskId}/fail"
	DaemonRouteTaskCancelAck            = "/api/daemon/tasks/{taskId}/cancel-ack"
	DaemonRouteTaskSession              = "/api/daemon/tasks/{taskId}/session"
	DaemonRouteWorkspaceIssueGCChecks   = "/api/daemon/workspaces/{workspaceId}/issues/gc-check"
	DaemonRouteIssueGCCheck             = "/api/daemon/issues/{issueId}/gc-check"
	DaemonRouteChatSessionGCCheck       = "/api/daemon/chat-sessions/{sessionId}/gc-check"
	DaemonRouteTaskGCCheck              = "/api/daemon/tasks/{taskId}/gc-check"
	DaemonRouteRuntimeRecoverOrphans    = "/api/daemon/runtimes/{runtimeId}/recover-orphans"
)

var lightweightDaemonRoutes = [...]LightweightDaemonRoute{
	{Method: http.MethodPost, Path: DaemonRouteRegister},
	{Method: http.MethodPost, Path: DaemonRouteDeregister},
	{Method: http.MethodPost, Path: DaemonRouteHeartbeat},
	{Method: http.MethodGet, Path: DaemonRouteWebSocket},
	{Method: http.MethodGet, Path: DaemonRouteWorkspaces},
	{Method: http.MethodGet, Path: DaemonRouteWorkspaceRepos},
	{Method: http.MethodGet, Path: DaemonRouteWorkspaceRuntimeProfiles},
	{Method: http.MethodPost, Path: DaemonRouteTasksClaim},
	{Method: http.MethodPost, Path: DaemonRouteTaskPrepareLease},
	{Method: http.MethodPost, Path: DaemonRouteTaskSkillBundlesResolve},
	{Method: http.MethodGet, Path: DaemonRouteRuntimePendingTasks},
	{Method: http.MethodPost, Path: DaemonRouteModelListResult},
	{Method: http.MethodPost, Path: DaemonRouteLocalSkillListResult},
	{Method: http.MethodPost, Path: DaemonRouteLocalSkillImportResult},
	{Method: http.MethodGet, Path: DaemonRouteTaskStatus},
	{Method: http.MethodPost, Path: DaemonRouteTaskStart},
	{Method: http.MethodPost, Path: DaemonRouteTaskWaitLocalDirectory},
	{Method: http.MethodPost, Path: DaemonRouteTaskProgress},
	{Method: http.MethodPost, Path: DaemonRouteTaskMessages},
	{Method: http.MethodGet, Path: DaemonRouteTaskMessages},
	{Method: http.MethodPost, Path: DaemonRouteTaskUsage},
	{Method: http.MethodPost, Path: DaemonRouteTaskComplete},
	{Method: http.MethodPost, Path: DaemonRouteTaskFail},
	{Method: http.MethodPost, Path: DaemonRouteTaskCancelAck},
	{Method: http.MethodPost, Path: DaemonRouteTaskSession},
	{Method: http.MethodPost, Path: DaemonRouteWorkspaceIssueGCChecks},
	{Method: http.MethodGet, Path: DaemonRouteIssueGCCheck},
	{Method: http.MethodGet, Path: DaemonRouteChatSessionGCCheck},
	{Method: http.MethodGet, Path: DaemonRouteTaskGCCheck},
	{Method: http.MethodPost, Path: DaemonRouteRuntimeRecoverOrphans},
}

// LightweightDaemonRoutes returns a copy of the frozen daemon route manifest.
func LightweightDaemonRoutes() []LightweightDaemonRoute {
	return append([]LightweightDaemonRoute(nil), lightweightDaemonRoutes[:]...)
}

// ExpandLightweightRoute substitutes escaped path parameters in one frozen
// route template. Callers pass alternating parameter names and values.
func ExpandLightweightRoute(template string, nameValues ...string) string {
	if len(nameValues)%2 != 0 {
		panic("ExpandLightweightRoute requires name/value pairs")
	}
	path := template
	for index := 0; index < len(nameValues); index += 2 {
		placeholder := "{" + nameValues[index] + "}"
		path = strings.ReplaceAll(path, placeholder, url.PathEscape(nameValues[index+1]))
	}
	return path
}

var lightweightWebEvents = [...]string{
	EventWorkspaceUpdated,
	EventWorkspaceDeleted,
	EventAgentCreated,
	EventAgentUpdated,
	EventAgentStatus,
	EventAgentArchived,
	EventAgentRestored,
	EventSkillCreated,
	EventSkillUpdated,
	EventSkillDeleted,
	EventSquadCreated,
	EventSquadUpdated,
	EventSquadDeleted,
	EventIssueCreated,
	EventIssueUpdated,
	EventIssueDeleted,
	EventCommentCreated,
	EventTaskQueued,
	EventTaskDispatch,
	EventTaskWaitingLocalDirectory,
	EventTaskRunning,
	EventTaskProgress,
	EventTaskMessage,
	EventTaskCompleted,
	EventTaskFailed,
	EventTaskCancelled,
	EventChatMessage,
	EventChatDone,
	EventChatCancelFinalized,
	EventChatSessionUpdated,
	EventChatSessionDeleted,
	EventDaemonRegister,
}

var lightweightDaemonControlEvents = [...]string{
	EventDaemonHeartbeat,
	EventDaemonHeartbeatAck,
	EventDaemonTaskAvailable,
	EventDaemonRuntimeProfilesChanged,
	EventDaemonWorkspacesChanged,
	EventDaemonPendingWork,
	EventDaemonRPCRequest,
	EventDaemonRPCResponse,
}

var lightweightWebEventSet = eventSet(lightweightWebEvents[:])
var lightweightDaemonControlEventSet = eventSet(lightweightDaemonControlEvents[:])

func eventSet(events []string) map[string]struct{} {
	set := make(map[string]struct{}, len(events))
	for _, event := range events {
		set[event] = struct{}{}
	}
	return set
}

// LightweightWebEvents returns a copy of the only event types valid on /ws.
func LightweightWebEvents() []string {
	return append([]string(nil), lightweightWebEvents[:]...)
}

// LightweightDaemonControlEvents returns a copy of the only event types valid
// on /api/daemon/ws.
func LightweightDaemonControlEvents() []string {
	return append([]string(nil), lightweightDaemonControlEvents[:]...)
}

func IsLightweightWebEvent(event string) bool {
	_, ok := lightweightWebEventSet[event]
	return ok
}

func IsLightweightDaemonControlEvent(event string) bool {
	_, ok := lightweightDaemonControlEventSet[event]
	return ok
}
