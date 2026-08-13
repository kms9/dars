"use client";

import { use, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { WorkspaceSlugProvider, paths } from "@dars/core/paths";
import { workspaceBySlugOptions } from "@dars/core/workspace";
import { setCurrentWorkspace } from "@dars/core/platform";
import { useAuthStore } from "@dars/core/auth";

export default function WorkspaceLayout({ children, params }: { children: React.ReactNode; params: Promise<{ workspaceSlug: string }> }) {
  const { workspaceSlug } = use(params);
  const user = useAuthStore((state) => state.user);
  const authLoading = useAuthStore((state) => state.isLoading);
  const router = useRouter();
  const { data: workspace, isFetched } = useQuery({ ...workspaceBySlugOptions(workspaceSlug), enabled: !!user });

  useEffect(() => {
    if (!authLoading && !user) router.replace(paths.login());
  }, [authLoading, router, user]);

  if (workspace) setCurrentWorkspace(workspace.slug, workspace.id);

  useEffect(() => {
    if (!workspace) return;
    const secure = location.protocol === "https:" ? "; Secure" : "";
    document.cookie = `last_workspace_slug=${encodeURIComponent(workspaceSlug)}; path=/; max-age=31536000; SameSite=Lax${secure}`;
  }, [workspace, workspaceSlug]);

  if (authLoading || (!!user && !isFetched)) return <div className="grid h-svh place-items-center text-body text-muted-foreground">Loading…</div>;
  if (!user) return null;
  if (!workspace) return <main className="grid h-svh place-items-center p-6 text-center"><div><h1 className="text-title-lg font-semibold">Workspace unavailable</h1><p className="mt-2 text-body text-muted-foreground">This workspace does not exist or is not available to your account.</p><button className="mt-4 rounded border px-4 py-2" onClick={() => router.replace(paths.newWorkspace())}>Choose a workspace</button></div></main>;
  return <WorkspaceSlugProvider slug={workspaceSlug}>{children}</WorkspaceSlugProvider>;
}
