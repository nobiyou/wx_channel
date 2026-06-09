import { NextRequest, NextResponse } from "next/server";

import { HubUnauthorizedError, getSubscriptionVideos } from "@/lib/hub";
import { getOptionalHubToken } from "@/lib/session";

export async function GET(
  request: NextRequest,
  context: { params: Promise<{ id: string }> }
) {
  const token = await getOptionalHubToken();
  if (!token) {
    return NextResponse.json({ error: "未登录" }, { status: 401 });
  }

  const { id } = await context.params;
  const page = Number(request.nextUrl.searchParams.get("page") || "1");

  try {
    const data = await getSubscriptionVideos(Number(id), page, token);
    return NextResponse.json({ data });
  } catch (error) {
    if (error instanceof HubUnauthorizedError) {
      return NextResponse.json({ error: "登录已失效" }, { status: 401 });
    }
    return NextResponse.json({ error: "读取订阅视频失败" }, { status: 500 });
  }
}
