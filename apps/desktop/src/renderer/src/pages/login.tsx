import { LoginPage } from "@dars/views/auth";
import { DragStrip } from "@dars/views/platform";
import { DARSIcon } from "@dars/ui/components/common/dars-icon";

function requireRuntimeAppUrl(): string {
  const runtimeConfig = window.desktopAPI.runtimeConfig;
  if (!runtimeConfig.ok) {
    throw new Error(
      "Invariant violated: DesktopLoginPage rendered before App accepted runtime config",
    );
  }
  return runtimeConfig.config.appUrl;
}

export function DesktopLoginPage() {
  const webUrl = requireRuntimeAppUrl();
  const handleGoogleLogin = () => {
    // Open web login page in the default browser with platform=desktop flag.
    // The web callback will redirect back via dars:// deep link with the token.
    window.desktopAPI.openExternal(
      `${webUrl}/login?platform=desktop`,
    );
  };

  return (
    <div className="flex h-screen flex-col">
      <DragStrip />
      <LoginPage
        logo={<DARSIcon bordered size="lg" />}
        onSuccess={() => {
          // Auth store update triggers AppContent re-render → shows DesktopShell.
          // Initial workspace navigation happens in routes.tsx via IndexRedirect.
        }}
        onGoogleLogin={handleGoogleLogin}
      />
    </div>
  );
}
