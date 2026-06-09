import { NextRequest, NextResponse } from "next/server";

import { HUB_TOKEN_COOKIE, HubConfigError, getHubApiBase } from "@/lib/hub";

type LoginResponse = {
  token: string;
  user: {
    id?: number;
    email?: string;
    username?: string;
    role?: string;
  };
};

export async function POST(request: NextRequest) {
  const body = await request.json().catch(() => null);
  if (!body?.email || !body?.password) {
    return NextResponse.json({ error: "邮箱和密码不能为空" }, { status: 400 });
  }

  let apiBase = "";
  try {
    apiBase = getHubApiBase();
  } catch (error) {
    if (error instanceof HubConfigError) {
      return NextResponse.json({ error: "HUB_API_BASE 未配置" }, { status: 503 });
    }
    throw error;
  }

  const upstream = await fetch(`${apiBase}/api/auth/login`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json"
    },
    body: JSON.stringify({
      email: body.email,
      password: body.password
    }),
    cache: "no-store"
  });

  if (!upstream.ok) {
    const text = await upstream.text();
    return NextResponse.json(
      { error: text || "登录失败" },
      { status: upstream.status }
    );
  }

  const payload = (await upstream.json()) as LoginResponse;
  const response = NextResponse.json({
    ok: true,
    user: payload.user
  });

  response.cookies.set(HUB_TOKEN_COOKIE, payload.token, {
    httpOnly: true,
    sameSite: "lax",
    secure: request.nextUrl.protocol === "https:",
    path: "/",
    maxAge: 60 * 60 * 24
  });

  return response;
}
