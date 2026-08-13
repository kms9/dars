"use client";

import { use } from "react";
import { ToolSourceDetailPage } from "@dars/views/lightweight";

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  return <ToolSourceDetailPage id={use(params).id} />;
}
