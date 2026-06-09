import "server-only";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { HUB_TOKEN_COOKIE } from "@/lib/hub";

export async function getOptionalHubToken(): Promise<string | null> {
  const cookieStore = await cookies();
  return cookieStore.get(HUB_TOKEN_COOKIE)?.value || null;
}

export async function requireHubToken(): Promise<string> {
  const token = await getOptionalHubToken();
  if (!token) {
    redirect("/login");
  }
  return token;
}
