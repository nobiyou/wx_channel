import { NextResponse } from "next/server";

import { HubUnauthorizedError, getProfile } from "@/lib/hub";
import { getOptionalHubToken } from "@/lib/session";

export async function GET() {
  const token = await getOptionalHubToken();
  if (!token) {
    return NextResponse.json({ error: "未登录" }, { status: 401 });
  }

  try {
    const profile = await getProfile(token);
    return NextResponse.json(profile);
  } catch (error) {
    if (error instanceof HubUnauthorizedError) {
      return NextResponse.json({ error: "登录已失效" }, { status: 401 });
    }
    return NextResponse.json({ error: "读取用户资料失败" }, { status: 500 });
  }
}
