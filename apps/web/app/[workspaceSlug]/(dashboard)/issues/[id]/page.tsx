"use client";

import { use } from "react";
import { RunDetailPage } from "@dars/views/lightweight";

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  return <RunDetailPage id={use(params).id} />;
}
