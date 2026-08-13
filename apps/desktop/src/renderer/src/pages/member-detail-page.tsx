import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { MemberDetailPage as SharedMemberDetailPage } from "@dars/views/members";
import { useWorkspaceId } from "@dars/core/hooks";
import { memberListOptions } from "@dars/core/workspace/queries";
import { useDocumentTitle } from "@/hooks/use-document-title";

export function MemberDetailPage() {
  const { id } = useParams<{ id: string }>();
  const wsId = useWorkspaceId();
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const member = members.find((m) => m.user_id === id) ?? null;

  useDocumentTitle(member?.name ?? "Member");

  if (!id) return null;
  return <SharedMemberDetailPage userId={id} />;
}
