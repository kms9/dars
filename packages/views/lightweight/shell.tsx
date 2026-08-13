"use client";
import type { ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Bot, BookOpenText, ListTodo, LogOut, MessageSquare, Monitor, Settings, Users, Wrench } from "lucide-react";
import { Button } from "@dars/ui/components/ui/button";
import { cn } from "@dars/ui/lib/utils";
import { useAuthStore } from "@dars/core/auth";
import { lightweightApi, lightweightKeys } from "@dars/core/lightweight";
import { paths, useCurrentWorkspace, useWorkspacePaths } from "@dars/core/paths";
import { AppLink, useNavigation } from "../navigation";

const nav = [
  ["runs", "Runs", ListTodo],
  ["chat", "Chat", MessageSquare],
  ["agents", "Agents", Bot],
  ["squads", "Squads", Users],
  ["skills", "Skills", BookOpenText],
  ["tools", "Tools", Wrench],
  ["runtimes", "Runtimes", Monitor],
  ["settings", "Settings", Settings],
] as const;

export function LightweightShell({ children }: { children: ReactNode }) {
  const workspace = useCurrentWorkspace();
  const workspacePaths = useWorkspacePaths();
  const { pathname, push } = useNavigation();
  const logout = useAuthStore((state) => state.logout);
  const user = useAuthStore((state) => state.user);
  const members = useQuery({
    queryKey: lightweightKeys.members(workspace?.id ?? ""),
    queryFn: () => lightweightApi.listMembers(workspace!.id),
    enabled: !!workspace,
  });
  const role = members.data?.find((member) => member.user_id === user?.id)?.role ?? null;
  const canManageTools = role === "owner" || role === "admin";

  if (!workspace) {
    return <div className="grid h-svh place-items-center text-body text-muted-foreground">Loading…</div>;
  }

  return (
    <div className="flex h-svh bg-background">
        <aside className="flex w-60 shrink-0 flex-col border-r bg-muted/20 p-3">
          <div className="mb-4 px-2 py-2">
            <div className="text-body font-semibold">DARS Lightweight</div>
            <div className="mt-1 truncate text-caption text-muted-foreground">{workspace?.name}</div>
          </div>
          <nav className="grid gap-1" aria-label="Workspace navigation">
            {nav.filter(([key]) => key !== "tools" || canManageTools).map(([key, label, Icon]) => {
              const href = workspacePaths[key]();
              const active = pathname === href || pathname.startsWith(`${href}/`);
              return (
                <AppLink
                  key={key}
                  href={href}
                  className={cn(
                    "flex items-center gap-2 rounded-md px-3 py-2 text-body text-muted-foreground hover:bg-muted hover:text-foreground",
                    active && "bg-muted font-medium text-foreground",
                  )}
                >
                  <Icon className="size-4" />
                  {label}
                </AppLink>
              );
            })}
          </nav>
          <div className="mt-auto grid gap-2 border-t pt-3">
            <Button variant="ghost" className="justify-start" onClick={() => push(paths.newWorkspace())}>Switch workspace</Button>
            <Button
              variant="ghost"
              className="justify-start text-muted-foreground"
              onClick={() => {
                logout();
                push(paths.login());
              }}
            >
              <LogOut className="size-4" />
              Sign out
            </Button>
          </div>
        </aside>
        <div className="min-w-0 flex-1">{children}</div>
    </div>
  );
}
