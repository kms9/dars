import "@testing-library/jest-dom/vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import LoginPageClient from "./login-client";

const autoLogin = {
  enabled: true,
  email: "dev@local.test",
  code: "424242",
} as const;

const { navigate, auth, listWorkspaces, createWorkspace } = vi.hoisted(() => ({
  navigate: { replace: vi.fn() },
  auth: {
    user: null as null | { id: string },
    isLoading: false,
    sendCode: vi.fn<() => Promise<void>>(),
    verifyCode: vi.fn<() => Promise<Record<string, unknown>>>(),
    refreshMe: vi.fn<() => Promise<void>>(),
  },
  listWorkspaces: vi.fn<() => Promise<Array<{ slug: string }>>>(),
  createWorkspace: vi.fn<() => Promise<{ slug: string }>>(),
}));

vi.mock("@dars/views/navigation", () => ({
  useNavigation: () => navigate,
}));

vi.mock("@dars/core/auth", () => ({
  useAuthStore: (selector: (state: typeof auth) => unknown) => selector(auth),
}));

vi.mock("@dars/core/lightweight", () => ({
  lightweightApi: { listWorkspaces, createWorkspace },
}));

describe("Lightweight email login", () => {
  beforeEach(() => {
    auth.user = null;
    auth.isLoading = false;
    auth.sendCode.mockReset().mockResolvedValue(undefined);
    auth.verifyCode.mockReset().mockImplementation(async () => {
      auth.user = { id: "user-1" };
      return {};
    });
    auth.refreshMe.mockReset().mockResolvedValue(undefined);
    listWorkspaces.mockReset().mockResolvedValue([]);
    createWorkspace.mockReset().mockResolvedValue({ slug: "demo" });
    navigate.replace.mockReset();
  });

  it("bootstraps development auto-login without showing the manual form", async () => {
    render(<LoginPageClient autoLogin={autoLogin} />);
    expect(screen.getByRole("heading", { name: "Signing you in" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Email")).not.toBeInTheDocument();
    expect(screen.queryByText(/Google/i)).not.toBeInTheDocument();
    await waitFor(() => expect(auth.sendCode).toHaveBeenCalledWith("dev@local.test"));
  });

  it("auto-logs in with the default development user and enters the first workspace", async () => {
    listWorkspaces.mockResolvedValue([{ slug: "acme" }]);
    render(<LoginPageClient autoLogin={autoLogin} />);
    await waitFor(() => expect(auth.sendCode).toHaveBeenCalledWith("dev@local.test"));
    await waitFor(() => expect(auth.verifyCode).toHaveBeenCalledWith("dev@local.test", "424242"));
    await waitFor(() => expect(navigate.replace).toHaveBeenCalledWith("/acme/issues"));
    expect(createWorkspace).not.toHaveBeenCalled();
  });

  it("auto-creates the demo workspace when none exist", async () => {
    render(<LoginPageClient autoLogin={autoLogin} />);
    await waitFor(() => expect(createWorkspace).toHaveBeenCalledWith({ name: "Demo", slug: "demo" }));
    await waitFor(() => expect(navigate.replace).toHaveBeenCalledWith("/demo/issues"));
  });

  it("allows manual send and verify after auto-login fails", async () => {
    const user = userEvent.setup();
    auth.sendCode.mockRejectedValueOnce(new Error("auto failed")).mockResolvedValue(undefined);
    listWorkspaces.mockResolvedValue([{ slug: "acme" }]);
    render(<LoginPageClient autoLogin={autoLogin} />);
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("auto failed"));
    await user.clear(screen.getByLabelText("Email"));
    await user.type(screen.getByLabelText("Email"), "dev@example.com");
    await user.click(screen.getByRole("button", { name: "Send code" }));
    expect(auth.sendCode).toHaveBeenLastCalledWith("dev@example.com");
    const codeInput = await screen.findByLabelText("Verification code");
    await user.clear(codeInput);
    await user.type(codeInput, "123456");
    await user.click(screen.getByRole("button", { name: "Verify and continue" }));
    await waitFor(() => expect(navigate.replace).toHaveBeenCalledWith("/acme/issues"));
  });

  it("keeps manual login when the server-side development override is disabled", () => {
    render(
      <LoginPageClient
        autoLogin={{ enabled: false, email: "dev@local.test", code: "" }}
      />,
    );

    expect(screen.getByRole("heading", { name: "Sign in with email" })).toBeInTheDocument();
    expect(screen.getByLabelText("Email")).toBeInTheDocument();
    expect(auth.sendCode).not.toHaveBeenCalled();
  });
});
