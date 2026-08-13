import { describe, expect, it } from "vitest";
import { resolveDevAutoLoginConfig } from "./dev-auto-login";

describe("resolveDevAutoLoginConfig", () => {
  it("enables auto-login with the configured development verification code", () => {
    expect(
      resolveDevAutoLoginConfig({
        APP_ENV: "development",
        DARS_DEV_VERIFICATION_CODE: "888888",
      }),
    ).toEqual({
      enabled: true,
      email: "dev@local.test",
      code: "888888",
    });
  });

  it("disables auto-login in production even when a code is configured", () => {
    expect(
      resolveDevAutoLoginConfig({
        APP_ENV: "production",
        DARS_DEV_VERIFICATION_CODE: "424242",
      }),
    ).toEqual({
      enabled: false,
      email: "dev@local.test",
      code: "",
    });
  });

  it.each([undefined, "", "12345", "abcdef"])(
    "disables auto-login for an absent or invalid code: %s",
    (code) => {
      expect(
        resolveDevAutoLoginConfig({
          APP_ENV: "development",
          DARS_DEV_VERIFICATION_CODE: code,
        }).enabled,
      ).toBe(false);
    },
  );
});
