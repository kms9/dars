"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@dars/core/auth";
import { ApiError } from "@dars/core/api";
import { useNavigation } from "../navigation";
import {
  canInvokeAgent,
  canManageAgent,
  canViewAgentSettings,
  lightweightAgentDetailOptions,
  lightweightAgentSnapshotOptions,
  lightweightAgentTasksOptions,
  lightweightApi,
  lightweightKeys,
  type AgentCapabilitiesSubview,
  type AgentDetailView,
} from "@dars/core/lightweight";
import { useCurrentWorkspace, useWorkspacePaths } from "@dars/core/paths";
import { Button } from "@dars/ui/components/ui/button";
import { AppLink } from "../navigation";
import { AgentDetailCapabilitiesPanel } from "./agents-detail-capabilities";
import {
  useAgentDetailReconnectRefetch,
  useAgentDetailViewState,
  useDirtyNavigationGuard,
  type DirtyPromptState,
} from "./agents-detail-hooks";
import { AgentDetailOverviewPanel, AgentDetailWorkPanel } from "./agents-detail-overview";
import { AgentDetailSettingsPanel } from "./agents-detail-settings";
import { ErrorState, PageFrame, Panel } from "./components";

function DirtyPrompt({ prompt }: { prompt: DirtyPromptState }) {
  return (
    <Panel className="border-amber-500/40 bg-amber-500/5">
      <p className="text-body">{prompt.message}</p>
      <div className="mt-3 flex flex-wrap gap-2">
        <Button size="sm" onClick={() => void prompt.onSave()}>Save</Button>
        <Button size="sm" variant="outline" onClick={prompt.onDiscard}>Discard</Button>
        <Button size="sm" variant="ghost" onClick={prompt.onCancel}>Cancel</Button>
      </div>
    </Panel>
  );
}

const VISIBLE_VIEWS: Array<{ id: AgentDetailView; label: string; manageOnly?: boolean }> = [
  { id: "overview", label: "Overview" },
  { id: "work", label: "Work" },
  { id: "capabilities", label: "Capabilities" },
  { id: "settings", label: "Settings" },
];

export function AgentDetailPage({ id }: { id: string }) {
  const workspace = useCurrentWorkspace();
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const user = useAuthStore((state) => state.user);
  const queryClient = useQueryClient();
  const workspaceId = workspace?.id ?? "";
  const { state, setView, setState } = useAgentDetailViewState();
  const [instructions, setInstructions] = useState("");
  const [instructionsDirty, setInstructionsDirty] = useState(false);
  const [actionError, setActionError] = useState<unknown>(null);

  const members = useQuery({
    queryKey: lightweightKeys.members(workspaceId),
    queryFn: () => lightweightApi.listMembers(workspace!.id),
    enabled: !!workspace,
  });
  const memberRole = members.data?.find((member) => member.user_id === user?.id)?.role ?? null;
  const currentMemberId = members.data?.find((member) => member.user_id === user?.id)?.id ?? null;

  const agent = useQuery(lightweightAgentDetailOptions(workspaceId, id));
  const snapshot = useQuery(lightweightAgentSnapshotOptions(workspaceId));
  const tasks = useQuery(lightweightAgentTasksOptions(workspaceId, id));
  const runtimes = useQuery({
    queryKey: lightweightKeys.runtimes(workspaceId),
    queryFn: lightweightApi.listRuntimes,
    enabled: !!workspace,
  });
  const chats = useQuery({
    queryKey: lightweightKeys.chats(workspaceId),
    queryFn: () => lightweightApi.listChatSessions(),
    enabled: !!workspace,
  });

  useAgentDetailReconnectRefetch(workspaceId, id);

  useEffect(() => {
    if (!agent.data) return;
    setInstructions(agent.data.instructions);
    setInstructionsDirty(false);
  }, [agent.data?.id, agent.data?.instructions, agent.data?.updated_at]);

  const runtime = useMemo(
    () => runtimes.data?.find((item) => item.id === agent.data?.runtime_id) ?? null,
    [agent.data?.runtime_id, runtimes.data],
  );

  const snapshotTasks = useMemo(() => {
    return snapshot.data?.tasks.filter((task) => task.agent_id === id) ?? [];
  }, [id, snapshot.data?.tasks]);

  const ownerName = useMemo(() => {
    const owner = snapshot.data?.filter_metadata.owners.find((item) => item.id === agent.data?.owner_id);
    return owner?.name ?? agent.data?.owner_id ?? "—";
  }, [agent.data?.owner_id, snapshot.data?.filter_metadata.owners]);

  const canManage = agent.data
    ? canManageAgent(agent.data.owner_id, user?.id ?? null, memberRole)
    : false;
  const canInvoke = agent.data
    ? canInvokeAgent(agent.data, agent.data.invocation_targets, user?.id ?? null, currentMemberId)
    : false;
  const canRevealEnv = agent.data
    ? canViewAgentSettings(agent.data, user?.id ?? null, memberRole)
    : false;
  const canManageTools = memberRole === "owner" || memberRole === "admin";

  const refreshAgent = () => {
    void queryClient.invalidateQueries({ queryKey: lightweightKeys.agents(workspaceId) });
    void queryClient.invalidateQueries({ queryKey: ["lightweight", workspaceId, "agent-detail", id] });
    void queryClient.invalidateQueries({ queryKey: ["lightweight", workspaceId, "agent-snapshot"] });
    void queryClient.invalidateQueries({ queryKey: ["lightweight", workspaceId, "agent-detail", id, "tasks"] });
  };

  const saveInstructions = useMutation({
    mutationFn: () => lightweightApi.updateAgent(id, { instructions }),
    onSuccess: () => {
      setInstructionsDirty(false);
      refreshAgent();
    },
  });

  const dirtyGuard = useDirtyNavigationGuard({
    dirty: instructionsDirty && state.view === "capabilities" && state.cap === "instructions",
    onSave: async () => {
      await saveInstructions.mutateAsync();
    },
    onDiscard: () => {
      setInstructions(agent.data?.instructions ?? "");
      setInstructionsDirty(false);
    },
  });

  const cancelWork = useMutation({
    mutationFn: () => lightweightApi.cancelAgentTasks(id),
    onSuccess: refreshAgent,
  });

  const assignWork = useMutation({
    mutationFn: async () => {
      if (!agent.data) throw new Error("Agent not loaded");
      return lightweightApi.createRun({
        title: `Work for ${agent.data.name}`,
        description: "",
        status: "todo",
        assignee_type: "agent",
        assignee_id: agent.data.id,
        acceptance_criteria: [],
        context_refs: [],
      });
    },
    onSuccess: (run) => {
      setActionError(null);
      navigation.push(paths.issueDetail(run.id));
    },
    onError: (error) => setActionError(error),
  });

  const openDirectChat = async () => {
    if (!agent.data) return;
    setActionError(null);
    try {
      const existing = chats.data?.items.find((session) => session.agent_id === agent.data.id && session.status === "active");
      if (existing) {
        navigation.push(`${paths.chat()}?session=${existing.id}`);
        return;
      }
      const created = await lightweightApi.createChatSession(agent.data.id, `Chat with ${agent.data.name}`);
      navigation.push(`${paths.chat()}?session=${created.id}`);
    } catch (error) {
      setActionError(error);
    }
  };

  const handleDirectMessage = () => {
    if (!canInvoke) {
      setActionError(new ApiError("forbidden", 403, "You cannot message this agent"));
      return;
    }
    if (agent.data?.archived_at) {
      setActionError(new ApiError("agent_archived", 409, "Agent is archived"));
      return;
    }
    if (!agent.data?.runtime_id || runtime?.status !== "online") {
      setActionError(new ApiError("agent_runtime_required", 400, "Runtime is unavailable"));
      return;
    }
    void openDirectChat();
  };

  const handleAssignWork = () => {
    if (!canInvoke) {
      setActionError(new ApiError("forbidden", 403, "You cannot assign work to this agent"));
      return;
    }
    if (agent.data?.archived_at) {
      setActionError(new ApiError("agent_archived", 409, "Agent is archived"));
      return;
    }
    if (!agent.data?.runtime_id || runtime?.status !== "online") {
      setActionError(new ApiError("agent_runtime_required", 400, "Runtime is unavailable"));
      return;
    }
    assignWork.mutate();
  };

  const requestViewChange = (view: AgentDetailView, extra?: Parameters<typeof setView>[1]) => {
    const go = () => setView(view, extra);
    if (view === state.view && !extra) return;
    if (instructionsDirty && state.view === "capabilities" && state.cap === "instructions") {
      dirtyGuard.requestNavigation(go, "Save instructions before leaving this tab?");
      return;
    }
    go();
  };

  if (!agent.data) {
    return (
      <PageFrame title="Agent">
        <ErrorState error={agent.error} />
        {agent.isPending ? <p>Loading…</p> : null}
      </PageFrame>
    );
  }

  const value = agent.data;
  const visibleViews = VISIBLE_VIEWS.filter((item) => !item.manageOnly || canManage);

  return (
    <PageFrame
      title={value.name}
      description="Overview, work history, capabilities, and settings."
      action={
        <AppLink href={paths.agents()} className="text-body text-primary underline">
          Back to agents
        </AppLink>
      }
    >
      {dirtyGuard.prompt ? <DirtyPrompt prompt={dirtyGuard.prompt} /> : null}

      <div className="flex flex-wrap gap-2 border-b pb-3">
        {visibleViews.map((item) => (
          <Button
            key={item.id}
            size="sm"
            variant={state.view === item.id ? "secondary" : "ghost"}
            onClick={() => requestViewChange(item.id)}
          >
            {item.label}
          </Button>
        ))}
      </div>

      {state.view === "overview" ? (
        <AgentDetailOverviewPanel
          agent={value}
          runtime={runtime}
          ownerName={ownerName}
          tasksPage={tasks.data}
          snapshotTasks={snapshotTasks}
          canManage={canManage}
          onCancelWork={() => cancelWork.mutate()}
          onDirectMessage={handleDirectMessage}
          onAssignWork={handleAssignWork}
          actionError={actionError ?? cancelWork.error ?? assignWork.error}
        />
      ) : null}

      {state.view === "work" ? (
        <AgentDetailWorkPanel
          agentId={id}
          workspaceId={workspaceId}
          state={state}
          onStateChange={(patch) => setState({ ...patch, view: "work" })}
        />
      ) : null}

      {state.view === "capabilities" ? (
        <AgentDetailCapabilitiesPanel
          agent={value}
          runtime={runtime}
          workspaceId={workspaceId}
          canManage={canManage}
          canManageTools={canManageTools}
          state={state}
          onCapChange={(cap: AgentCapabilitiesSubview) =>
            dirtyGuard.requestNavigation(
              () => setView("capabilities", { cap }),
              "Save instructions before switching capability tabs?",
            )
          }
          instructions={instructions}
          onInstructionsChange={(next) => {
            setInstructions(next);
            setInstructionsDirty(next !== value.instructions);
          }}
          instructionsDirty={instructionsDirty}
          onSaveInstructions={async () => {
            await saveInstructions.mutateAsync();
          }}
          instructionsError={saveInstructions.error}
          instructionsPending={saveInstructions.isPending}
        />
      ) : null}

      {state.view === "settings" ? (
        <AgentDetailSettingsPanel
          agent={value}
          runtimes={runtimes.data ?? []}
          workspaceId={workspaceId}
          canManage={canManage}
          canRevealEnv={canRevealEnv}
          members={members.data ?? []}
        />
      ) : null}
    </PageFrame>
  );
}
