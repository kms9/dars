"use client";

import { useEffect } from "react";
import { useAuthStore } from "@dars/core/auth";
import { paths } from "@dars/core/paths";
import { WorkspacePickerPage } from "@dars/views/lightweight";
import { useNavigation } from "@dars/views/navigation";

export default function Page() {
  const navigation = useNavigation();
  const user = useAuthStore((state) => state.user);
  const isLoading = useAuthStore((state) => state.isLoading);
  const refreshMe = useAuthStore((state) => state.refreshMe);
  useEffect(() => {
    if (!isLoading && !user) {
      // Email verification sets the HttpOnly cookie before this route is
      // loaded. A Next.js route-boundary transition can mount this page before
      // the module-level auth store carries its in-memory user across; recover
      // from the authoritative cookie before treating the visit as anonymous.
      void refreshMe().catch(() => navigation.replace(paths.login()));
    }
  }, [isLoading, navigation, refreshMe, user]);
  return user ? <WorkspacePickerPage /> : null;
}
