import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { AgentDetailPage as SharedAgentDetailPage } from "@dars/views/agents";
import { useWorkspaceId } from "@dars/core/hooks";
import { agentListOptions } from "@dars/core/workspace/queries";
import { useDocumentTitle } from "@/hooks/use-document-title";

export function AgentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const wsId = useWorkspaceId();
  const { data: agents = [] } = useQuery(agentListOptions(wsId));
  const agent = agents.find((a) => a.id === id) ?? null;

  useDocumentTitle(agent?.name ?? "Agent");

  if (!id) return null;
  return <SharedAgentDetailPage agentId={id} />;
}
