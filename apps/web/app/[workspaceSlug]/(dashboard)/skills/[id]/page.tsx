"use client";

import { use } from "react";
import { SkillDetailPage } from "@dars/views/lightweight";

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  return <SkillDetailPage id={use(params).id} />;
}
