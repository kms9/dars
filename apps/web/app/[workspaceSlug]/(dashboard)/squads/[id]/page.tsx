"use client";

import { use } from "react";
import { SquadDetailPage } from "@dars/views/lightweight";

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  return <SquadDetailPage id={use(params).id} />;
}
