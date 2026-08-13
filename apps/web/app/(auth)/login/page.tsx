import LoginPageClient from "./login-client";
import { getRuntimeDevAutoLoginConfig } from "@/platform/dev-auto-login";

export default async function LoginPage() {
  const autoLogin = await getRuntimeDevAutoLoginConfig();
  return <LoginPageClient autoLogin={autoLogin} />;
}
