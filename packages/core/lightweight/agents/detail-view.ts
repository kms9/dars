export type AgentDetailView = "overview" | "work" | "capabilities" | "settings";

export type AgentCapabilitiesSubview = "instructions" | "skills" | "tools";

export type AgentWorkScope = "assigned" | "created";

export type AgentDetailViewState = {
  view: AgentDetailView;
  cap: AgentCapabilitiesSubview;
  workScope: AgentWorkScope;
  workStatus: string;
  workQuery: string;
  workCursor: string | null;
};

const DETAIL_VIEWS = new Set<AgentDetailView>(["overview", "work", "capabilities", "settings"]);
const CAP_SUBVIEWS = new Set<AgentCapabilitiesSubview>(["instructions", "skills", "tools"]);
const WORK_SCOPES = new Set<AgentWorkScope>(["assigned", "created"]);
const RETIRED_VIEWS = new Set(["mcp", "integrations", "instructions", "skills"]);

export function parseAgentDetailViewState(searchParams: URLSearchParams): AgentDetailViewState {
  const rawView = (searchParams.get("view") ?? "overview").toLowerCase();
  let view: AgentDetailView = "overview";
  let cap: AgentCapabilitiesSubview = "instructions";

  if (RETIRED_VIEWS.has(rawView)) {
    if (rawView === "skills" || rawView === "instructions") {
      view = "capabilities";
      cap = rawView === "skills" ? "skills" : "instructions";
    } else {
      view = "overview";
    }
  } else if (DETAIL_VIEWS.has(rawView as AgentDetailView)) {
    view = rawView as AgentDetailView;
  }

  const rawCap = searchParams.get("cap");
  if (rawCap) {
    const normalizedCap = rawCap.toLowerCase();
    if (CAP_SUBVIEWS.has(normalizedCap as AgentCapabilitiesSubview)) {
      cap = normalizedCap as AgentCapabilitiesSubview;
    }
  }

  const rawScope = (searchParams.get("workScope") ?? "assigned").toLowerCase();
  const workScope = WORK_SCOPES.has(rawScope as AgentWorkScope)
    ? (rawScope as AgentWorkScope)
    : "assigned";

  return {
    view,
    cap,
    workScope,
    workStatus: searchParams.get("workStatus") ?? "",
    workQuery: searchParams.get("workQuery") ?? "",
    workCursor: searchParams.get("workCursor") || null,
  };
}

export function buildAgentDetailSearchParams(
  state: Partial<AgentDetailViewState> & Pick<AgentDetailViewState, "view">,
): URLSearchParams {
  const params = new URLSearchParams();
  if (state.view !== "overview") params.set("view", state.view);
  if (state.view === "capabilities" && state.cap && state.cap !== "instructions") {
    params.set("cap", state.cap);
  }
  if (state.view === "work") {
    if (state.workScope && state.workScope !== "assigned") params.set("workScope", state.workScope);
    if (state.workStatus) params.set("workStatus", state.workStatus);
    if (state.workQuery) params.set("workQuery", state.workQuery);
    if (state.workCursor) params.set("workCursor", state.workCursor);
  }
  return params;
}

export function agentDetailHref(pathname: string, state: Partial<AgentDetailViewState> & Pick<AgentDetailViewState, "view">): string {
  const params = buildAgentDetailSearchParams(state);
  const query = params.toString();
  return query ? `${pathname}?${query}` : pathname;
}
