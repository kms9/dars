import { connection } from "next/server";
import {
  resolveDevAutoLoginConfig,
  type DevAutoLoginConfig,
} from "@/config/dev-auto-login";

/** Read deployment environment variables at request time, including with `next start`. */
export async function getRuntimeDevAutoLoginConfig(): Promise<DevAutoLoginConfig> {
  await connection();
  return resolveDevAutoLoginConfig(process.env);
}
