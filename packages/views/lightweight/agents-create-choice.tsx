"use client";

import { useWorkspacePaths } from "@dars/core/paths";
import { Button } from "@dars/ui/components/ui/button";
import { useNavigation } from "../navigation";
import { PageFrame, Panel } from "./components";

export function ChooseCreateMethodPage() {
  const paths = useWorkspacePaths();
  const navigation = useNavigation();

  return (
    <PageFrame
      title="Create agent"
      description="Start from a blank configuration or continue with AI Builder in a later step."
      action={
        <Button variant="outline" onClick={() => navigation.push(paths.agents())}>
          Back to agents
        </Button>
      }
    >
      <Panel>
        <div className="grid gap-4 md:grid-cols-2">
          <button
            type="button"
            className="rounded-xl border bg-card p-6 text-left transition hover:border-primary/40 hover:bg-muted/30"
            onClick={() => navigation.push(paths.newAgentBlank())}
          >
            <div className="text-title-sm font-semibold">Start from blank</div>
            <p className="mt-2 text-body text-muted-foreground">
              Configure identity, instructions, skills, runtime, model, and access manually.
            </p>
          </button>
          <button
            type="button"
            className="rounded-xl border border-primary/30 bg-primary/[0.03] p-6 text-left transition hover:border-primary/40 hover:bg-muted/30"
            onClick={() => navigation.push(paths.newAgentAi())}
          >
            <div className="text-title-sm font-semibold">Create with AI</div>
            <p className="mt-2 text-body text-muted-foreground">
              Open a guided conversation, review the structured draft, and finalize into a real agent.
            </p>
          </button>
        </div>
      </Panel>
    </PageFrame>
  );
}
