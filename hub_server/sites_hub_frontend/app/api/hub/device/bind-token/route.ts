import { NextResponse } from "next/server";

import { HubActionResult, HubUnauthorizedError } from "@/lib/hub";
import { jsonError, proxyHubJson, requireSessionToken } from "@/app/api/hub/_utils";

export async function POST() {
  try {
    const token = await requireSessionToken();
    const payload = await proxyHubJson<HubActionResult>(
      "/api/device/bind_token",
      { method: "POST" },
      token
    );
    return NextResponse.json(payload);
  } catch (error) {
    if (error instanceof HubUnauthorizedError) {
      return jsonError("登录已失效", 401);
    }
    return jsonError(error instanceof Error ? error.message : "生成绑定码失败");
  }
}

