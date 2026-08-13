export interface DevAutoLoginConfig {
  enabled: boolean;
  email: string;
  code: string;
}

export const DEV_AUTO_LOGIN_EMAIL = "dev@local.test";

/**
 * Keep development auto-login aligned with the server-side verification-code
 * override. A production deployment must never expose or use the override.
 */
export function resolveDevAutoLoginConfig(
  env: Readonly<Record<string, string | undefined>>,
): DevAutoLoginConfig {
  const appEnv = env.APP_ENV?.trim().toLowerCase();
  const code = env.DARS_DEV_VERIFICATION_CODE?.trim() ?? "";
  const enabled = appEnv !== "production" && /^\d{6}$/.test(code);

  return {
    enabled,
    email: DEV_AUTO_LOGIN_EMAIL,
    code: enabled ? code : "",
  };
}
