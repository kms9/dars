"use client";

import { useEffect, useMemo, useState } from "react";
import { Button } from "@dars/ui/components/ui/button";
import { Input } from "@dars/ui/components/ui/input";
import { Textarea } from "@dars/ui/components/ui/textarea";
import type { LightweightSkillFile } from "@dars/core/lightweight";
import { ErrorState, Panel, SubmitButton } from "./components";

const SKILL_MD = "SKILL.md";

type FileMap = Map<string, string>;

function toMap(files: LightweightSkillFile[], skillContent: string): FileMap {
  const map = new Map<string, string>();
  for (const file of files) map.set(file.path, file.content);
  if (!map.has(SKILL_MD)) map.set(SKILL_MD, skillContent);
  return map;
}

function sortedPaths(map: FileMap): string[] {
  return [...map.keys()].sort((a, b) => {
    if (a === SKILL_MD) return -1;
    if (b === SKILL_MD) return 1;
    return a.localeCompare(b);
  });
}

export function SkillFilesEditor({
  skillContent,
  files,
  pending,
  error,
  onSave,
}: {
  skillContent: string;
  files: LightweightSkillFile[];
  pending?: boolean;
  error?: unknown;
  onSave: (files: Array<{ path: string; content: string }>, skillMdContent: string) => Promise<unknown>;
}) {
  const [fileMap, setFileMap] = useState<FileMap>(() => toMap(files, skillContent));
  const [selectedPath, setSelectedPath] = useState(SKILL_MD);
  const [newPath, setNewPath] = useState("");
  const [localError, setLocalError] = useState<string | null>(null);

  useEffect(() => {
    setFileMap(toMap(files, skillContent));
    setSelectedPath((prev) => (prev && toMap(files, skillContent).has(prev) ? prev : SKILL_MD));
  }, [files, skillContent]);

  const paths = useMemo(() => sortedPaths(fileMap), [fileMap]);
  const selectedContent = fileMap.get(selectedPath) ?? "";

  const updateContent = (content: string) => {
    setFileMap((prev) => {
      const next = new Map(prev);
      next.set(selectedPath, content);
      return next;
    });
  };

  const addFile = () => {
    const path = newPath.trim().replace(/^\/+/, "");
    if (!path) {
      setLocalError("Path is required");
      return;
    }
    if (path.includes("..") || path.startsWith("/")) {
      setLocalError("Path must be a relative path without traversal");
      return;
    }
    if (fileMap.has(path)) {
      setLocalError("A file with this path already exists");
      return;
    }
    setFileMap((prev) => new Map(prev).set(path, ""));
    setSelectedPath(path);
    setNewPath("");
    setLocalError(null);
  };

  const deleteFile = (path: string) => {
    if (path === SKILL_MD) return;
    setFileMap((prev) => {
      const next = new Map(prev);
      next.delete(path);
      return next;
    });
    if (selectedPath === path) setSelectedPath(SKILL_MD);
  };

  return (
    <Panel>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="font-semibold">Files</h2>
          <p className="mt-1 text-body text-muted-foreground">
            Edit the file tree, then save to replace the complete skill file set atomically.
          </p>
        </div>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-[220px_1fr]">
        <div className="rounded-lg border">
          <ul className="max-h-80 overflow-y-auto p-2 text-body">
            {paths.map((path) => (
              <li key={path}>
                <button
                  type="button"
                  onClick={() => setSelectedPath(path)}
                  className={`flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left hover:bg-muted/50 ${
                    selectedPath === path ? "bg-muted font-medium" : ""
                  }`}
                >
                  <span className="truncate">{path}</span>
                  {path === SKILL_MD ? <span className="text-caption text-muted-foreground">main</span> : null}
                </button>
              </li>
            ))}
          </ul>
          <div className="border-t p-2">
            <div className="flex gap-2">
              <Input
                value={newPath}
                onChange={(event) => setNewPath(event.target.value)}
                placeholder="docs/guide.md"
                aria-label="New file path"
              />
              <Button type="button" variant="outline" onClick={addFile}>Add</Button>
            </div>
          </div>
        </div>

        <div className="grid gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="font-mono text-caption text-muted-foreground">{selectedPath}</div>
            {selectedPath !== SKILL_MD ? (
              <Button type="button" variant="ghost" size="sm" onClick={() => deleteFile(selectedPath)}>
                Delete file
              </Button>
            ) : null}
          </div>
          <Textarea
            key={selectedPath}
            rows={16}
            value={selectedContent}
            onChange={(event) => updateContent(event.target.value)}
            className="font-mono text-body"
          />
        </div>
      </div>

      {localError ? <p role="alert" className="mt-3 text-body text-destructive">{localError}</p> : null}
      <ErrorState error={error} />
      <form
        className="mt-4"
        onSubmit={(event) => {
          event.preventDefault();
          const payload = paths.map((path) => ({ path, content: fileMap.get(path) ?? "" }));
          const skillMd = fileMap.get(SKILL_MD) ?? "";
          void onSave(payload, skillMd);
        }}
      >
        <SubmitButton pending={pending}>Save files</SubmitButton>
      </form>
    </Panel>
  );
}
