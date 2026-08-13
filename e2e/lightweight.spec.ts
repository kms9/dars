import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test, type Page } from "@playwright/test";

function buildSkillArchive(name: string): Buffer {
  const dir = mkdtempSync(join(tmpdir(), "lightweight-skill-"));
  const archivePath = join(dir, `${name}.skill`);
  execFileSync("python3", [
    "-c",
    [
      "import zipfile,sys",
      "name,path=sys.argv[1],sys.argv[2]",
      "content=f\"---\\nname: {name}\\ndescription: archive import\\n---\\n\\n# Archive skill\\n\"",
      "with zipfile.ZipFile(path,'w',compression=zipfile.ZIP_DEFLATED) as zf:",
      "    zf.writestr('SKILL.md', content)",
      "    zf.writestr('references/note.md', '# note\\n')",
    ].join("\n"),
    name,
    archivePath,
  ]);
  try {
    return readFileSync(archivePath);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

type ApiResult<T> = { status: number; body: T };

async function api<T>(
  page: Page,
  path: string,
  method = "GET",
  body?: unknown,
  workspaceId?: string,
): Promise<ApiResult<T>> {
  const result = await page.evaluate(
    async ({ path, method, body, workspaceId }) => {
      const csrf = document.cookie
        .split("; ")
        .find((entry) => entry.startsWith("dars_csrf="))
        ?.split("=")[1];
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (csrf) headers["X-CSRF-Token"] = csrf;
      if (workspaceId) headers["X-Workspace-ID"] = workspaceId;
      const response = await fetch(path, {
        method,
        headers,
        credentials: "include",
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      const text = await response.text();
      return {
        status: response.status,
        body: text ? JSON.parse(text) : null,
      };
    },
    { path, method, body, workspaceId },
  );
  expect(result.status, `${method} ${path}: ${JSON.stringify(result.body)}`).toBeGreaterThanOrEqual(200);
  expect(result.status, `${method} ${path}: ${JSON.stringify(result.body)}`).toBeLessThan(300);
  return result as ApiResult<T>;
}

test("Lightweight Web closes the Auth, Run, Chat, Squad and removed-route loop", async ({ page }) => {
  const suffix = `${Date.now().toString(36)}-${process.pid}`;
  const email = "dev@local.test";
  const slug = "demo";

  const verification = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/auth/verify-code" && response.status() === 200,
  );
  await page.goto("/");
  await verification;
  await expect(page).toHaveURL(new RegExp(`/${slug}/issues$`));
  await expect(page.getByRole("heading", { name: "Runs" })).toBeVisible();

  const nav = page.getByRole("navigation", { name: "Workspace navigation" });
  for (const label of ["Runs", "Chat", "Agents", "Squads", "Skills", "Tools", "Runtimes", "Settings"]) {
    await expect(nav.getByRole("link", { name: label, exact: true })).toBeVisible();
  }
  for (const removed of ["Projects", "Autopilots", "Inbox", "Billing", "Usage"]) {
    await expect(nav.getByRole("link", { name: removed, exact: true })).toHaveCount(0);
  }

  const workspaces = (await api<Array<{ id: string; slug: string }>>(page, "/api/workspaces")).body;
  const workspace = workspaces.find((item) => item.slug === slug);
  expect(workspace).toBeTruthy();
  const workspaceId = workspace!.id;

  const daemon = (await api<{
    token: string;
    runtimes: Array<{ id: string }>;
  }>(page, "/api/daemon/register", "POST", {
    protocol_version: "lightweight-runtime-v1",
    workspace_id: workspaceId,
    daemon_id: `e2e-daemon-${suffix}`,
    runtimes: [{ name: `E2E Codex ${suffix}`, type: "codex", version: "1" }],
  })).body;
  expect(daemon.token).toMatch(/^ddt_/);
  expect(daemon.runtimes).toHaveLength(1);

  const agent = (await api<{ id: string; name: string }>(page, "/api/agents", "POST", {
    name: "E2E Agent",
    runtime_id: daemon.runtimes[0]!.id,
    permission_mode: "public_to",
    invocation_targets: [{ target_type: "workspace", target_id: workspaceId }],
  }, workspaceId)).body;

  const secondAgent = (await api<{ id: string; name: string }>(page, "/api/agents", "POST", {
    name: "E2E Tool Agent",
    runtime_id: daemon.runtimes[0]!.id,
    permission_mode: "public_to",
    invocation_targets: [{ target_type: "workspace", target_id: workspaceId }],
  }, workspaceId)).body;

  const sourceName = `e2e-tools-${suffix}`.toLowerCase();
  const swagger = Buffer.from(JSON.stringify({
    swagger: "2.0",
    info: { title: "E2E Tools", version: "1" },
    paths: {
      "/pets": {
        get: { operationId: "listPets", responses: { 200: { description: "ok" } } },
        post: { operationId: "createPet", responses: { 200: { description: "ok" } } },
      },
    },
  }));
  await page.goto(`/${slug}/tools`);
  await expect(page.getByRole("heading", { name: "Tools", exact: true })).toBeVisible();
  await page.getByLabel("Canonical namespace", { exact: true }).fill(sourceName);
  await page.getByLabel("API base endpoint", { exact: true }).fill("https://8.8.8.8/api");
  await page.getByLabel("Artifact", { exact: true }).setInputFiles({
    name: "swagger.json",
    mimeType: "application/json",
    buffer: swagger,
  });
  const importRequestPromise = page.waitForRequest((request) => new URL(request.url()).pathname === "/api/tool-sources/import");
  await page.getByRole("button", { name: "Import and validate", exact: true }).click();
  const importRequest = await importRequestPromise;
  const importOrigin = new URL(importRequest.url()).origin;
  const allowedServerOrigins = new Set([
    new URL(page.url()).origin,
    `http://localhost:${process.env.PORT ?? "8080"}`,
    `http://127.0.0.1:${process.env.PORT ?? "8080"}`,
  ]);
  expect(allowedServerOrigins.has(importOrigin)).toBe(true);
  await expect(page.getByRole("heading", { name: sourceName, exact: true })).toBeVisible({ timeout: 15_000 });
  await page.getByRole("button", { name: "Enable", exact: true }).click();
  await expect(page.getByText("enabled", { exact: true }).first()).toBeVisible();

  const selectAgentTool = async (agentId: string, toolName: string, exportedName: string) => {
    await page.goto(`/${slug}/agents/${agentId}?view=capabilities&cap=tools`);
    const canonicalName = `${sourceName}.${toolName}`;
    const checkbox = page.getByRole("checkbox", { name: `Select ${canonicalName}`, exact: true });
    await expect(checkbox).toBeVisible({ timeout: 15_000 });
    await checkbox.check();
    await page.getByLabel(`Agent call name for ${canonicalName}`, { exact: true }).fill(exportedName);
    await page.getByRole("button", { name: "Publish 1 tools", exact: true }).click();
    const current = page.getByText(/tb_[A-Z0-9]+ · 1 tools · active/);
    await expect(current).toBeVisible();
    const match = (await current.textContent())?.match(/tb_[A-Z0-9]+/);
    expect(match?.[0]).toBeTruthy();
    return match![0];
  };

  const firstAlias = `skill.e2e.list_${suffix}`.toLowerCase();
  const firstBundleId = await selectAgentTool(agent.id, "listPets", firstAlias);
  await page.reload();
  await expect(page.getByText(firstBundleId, { exact: false })).toBeVisible();
  const restored = page.getByRole("checkbox", { name: `Select ${sourceName}.listPets`, exact: true });
  await expect(restored).toBeChecked();
  await expect(page.getByLabel(`Agent call name for ${sourceName}.listPets`, { exact: true })).toHaveValue(firstAlias);

  const secondBundleId = await selectAgentTool(secondAgent.id, "createPet", `skill.e2e.create_${suffix}`.toLowerCase());
  expect(secondBundleId).not.toBe(firstBundleId);

  await page.goto(`/${slug}/agents/${agent.id}?view=capabilities&cap=tools`);
  await page.getByRole("button", { name: "Clear current tools", exact: true }).click();
  await expect(page.getByText("No Server ToolBundle is assigned.", { exact: true })).toBeVisible();

  await page.goto(`/${slug}/squads`);
  await page.getByRole("button", { name: "Create squad", exact: true }).click();
  const squadName = `E2E Squad ${suffix}`;
  await page.getByRole("textbox", { name: "Name", exact: true }).fill(squadName);
  await page.getByRole("combobox", { name: "Leader agent", exact: true }).selectOption(agent.id);
  await page.getByRole("button", { name: "Create squad", exact: true }).last().evaluate((element) => {
    (element as HTMLButtonElement).click();
  });
  await expect(page.getByText(squadName, { exact: true })).toBeVisible();

  await page.goto(`/${slug}/issues`);
  const issueTitle = `E2E blocked run ${suffix}`;
  await page.getByLabel("Title").fill(issueTitle);
  await page.getByLabel("Assignee").selectOption(`agent:${agent.id}`);
  await page.getByLabel("Description").fill("Browser-created Lightweight Run");
  await page.getByLabel("Initial status").selectOption("backlog");
  await page.getByRole("button", { name: "Create run" }).click();
  await expect(page.getByText(issueTitle, { exact: true })).toBeVisible();
  await page.getByText(issueTitle, { exact: true }).click();
  await expect(page.getByRole("heading", { name: "Flat comments" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Task runs" })).toBeVisible();
  await page.getByPlaceholder("Add an append-only comment").fill("Browser evidence comment");
  await page.getByRole("button", { name: "Add comment" }).click();
  await expect(page.getByText("Browser evidence comment", { exact: true })).toBeVisible();
  await page.locator("select").first().selectOption("blocked");
  await expect(page.locator("select").first()).toHaveValue("blocked");

  await page.goto(`/${slug}/chat`);
  await page.getByLabel("Agent").selectOption(agent.id);
  const chatTitle = `E2E direct chat ${suffix}`;
  await page.getByLabel("Title").fill(chatTitle);
  await page.getByRole("button", { name: "New session" }).click();
  await expect(page.getByRole("heading", { name: chatTitle, exact: true })).toBeVisible();
  await page.getByPlaceholder("Message this agent").fill("hello from browser");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(page.getByText("hello from browser", { exact: true })).toBeVisible();
  await expect(page.getByText(/Task queued/)).toBeVisible();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();

  await page.goto(`/${slug}/runtimes`);
  await expect(page.getByText(`E2E Codex ${suffix}`, { exact: true })).toBeVisible();

  await page.goto(`/${slug}/skills`);
  await expect(page.getByRole("heading", { name: "Skills" })).toBeVisible();
  await page.getByTestId("create-method-manual").click();
  const skillName = `E2E Skill ${suffix}`;
  await page.getByLabel("Name").fill(skillName);
  await page.getByLabel("SKILL.md content").fill(`---\nname: ${skillName}\n---\n\n# E2E skill\n`);
  await page.getByRole("button", { name: "Create skill" }).click();
  await expect(page.getByRole("heading", { name: skillName })).toBeVisible({ timeout: 15_000 });

  const archiveName = `E2E Archive ${suffix}`;
  const archiveBytes = Array.from(buildSkillArchive(archiveName));
  const archiveImport = await page.evaluate(
    async ({ workspaceId, skillTitle, bytes }) => {
      const csrf = document.cookie
        .split("; ")
        .find((entry) => entry.startsWith("dars_csrf="))
        ?.split("=")[1];
      const form = new FormData();
      form.append("file", new Blob([new Uint8Array(bytes)], { type: "application/zip" }), `${skillTitle}.skill`);
      form.append("on_conflict", "fail");
      const headers: Record<string, string> = {};
      if (csrf) headers["X-CSRF-Token"] = csrf;
      if (workspaceId) headers["X-Workspace-ID"] = workspaceId;
      const response = await fetch("/api/skills/import", {
        method: "POST",
        headers,
        credentials: "include",
        body: form,
      });
      const text = await response.text();
      return { status: response.status, body: text ? JSON.parse(text) : null };
    },
    { workspaceId, skillTitle: archiveName, bytes: archiveBytes },
  );
  expect(archiveImport.status, JSON.stringify(archiveImport.body)).toBeGreaterThanOrEqual(200);
  expect(archiveImport.status, JSON.stringify(archiveImport.body)).toBeLessThan(300);
  expect(archiveImport.body?.status === "created" || archiveImport.body?.name).toBeTruthy();

  await page.goto(`/${slug}/skills`);
  await expect(page.getByText(archiveName, { exact: true })).toBeVisible({ timeout: 15_000 });

  await page.goto(`/${slug}/settings`).catch(async () => {
    await page.goto(`/${slug}/settings`);
  });
  await expect(page.getByText(email, { exact: true })).toBeVisible();

  for (const removedPath of ["projects", "autopilots", "inbox", "billing", "usage"]) {
    const response = await page.goto(`/${slug}/${removedPath}`, { waitUntil: "domcontentloaded" });
    expect(response?.status(), `removed deep link /${removedPath}`).toBe(404);
  }
});
