"use client";

import { use } from "react";
import { AgentDetailPage } from "@dars/views/lightweight";

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  return <AgentDetailPage id={use(params).id} />;
}
