import { NextRequest, NextResponse } from "next/server";

import { HubConfigError, HubUnauthorizedError, getHubApiBase, hubFetch } from "@/lib/hub";
import { getOptionalHubToken } from "@/lib/session";

export async function requireSessionToken() {
  const token = await getOptionalHubToken();
  if (!token) {
    throw new HubUnauthorizedError();
  }
  return token;
}

export function jsonError(message: string, status = 500) {
  return NextResponse.json({ ok: false, error: message }, { status });
}

export async function proxyHubJson<T>(
  path: string,
  init?: RequestInit,
  token?: string | null
): Promise<T> {
  return hubFetch<T>(path, init, token);
}

export async function proxyHubPassthrough(
  request: NextRequest,
  path: string,
  method: string
) {
  const token = await requireSessionToken();
  let apiBase = "";

  try {
    apiBase = getHubApiBase();
  } catch (error) {
    if (error instanceof HubConfigError) {
      return jsonError("HUB_API_BASE 未配置", 503);
    }
    throw error;
  }

  const bodyText =
    method === "GET" || method === "DELETE" ? undefined : await request.text().catch(() => "");

  const response = await fetch(`${apiBase}${path}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`
    },
    body: bodyText && bodyText.length > 0 ? bodyText : undefined,
    cache: "no-store"
  });

  const text = await response.text();

  return new NextResponse(text || JSON.stringify({ ok: response.ok }), {
    status: response.status,
    headers: {
      "Content-Type": response.headers.get("Content-Type") || "application/json"
    }
  });
}

