import { NextRequest, NextResponse } from "next/server";

import { HubUnauthorizedError, getSubscriptions } from "@/lib/hub";
import { getOptionalHubToken } from "@/lib/session";
import { proxyHubPassthrough } from "@/app/api/hub/_utils";

export async function GET() {
  const token = await getOptionalHubToken();
  if (!token) {
    return NextResponse.json({ error: "未登录" }, { status: 401 });
  }

  try {
    const subscriptions = await getSubscriptions(token);
    return NextResponse.json({ subscriptions });
  } catch (error) {
    if (error instanceof HubUnauthorizedError) {
      return NextResponse.json({ error: "登录已失效" }, { status: 401 });
    }
    return NextResponse.json({ error: "读取订阅列表失败" }, { status: 500 });
  }
}

export async function POST(request: NextRequest) {
  return proxyHubPassthrough(request, "/api/subscriptions", "POST");
}

