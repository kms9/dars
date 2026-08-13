"use client";

import { use } from "react";
import { AiBuilderSessionPage } from "@dars/views/lightweight";

export default function Page({ params }: { params: Promise<{ sessionId: string }> }) {
  const { sessionId } = use(params);
  return <AiBuilderSessionPage sessionId={sessionId} />;
}
