"use client";

import { use } from "react";
import { RuntimeDetailPage } from "@dars/views/lightweight";

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  return <RuntimeDetailPage id={use(params).id} />;
}
