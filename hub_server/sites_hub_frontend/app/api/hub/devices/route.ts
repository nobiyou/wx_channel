import { NextResponse } from "next/server";

import { HubUnauthorizedError, getClients, getUserDevices } from "@/lib/hub";
import { getOptionalHubToken } from "@/lib/session";

export async function GET() {
  const token = await getOptionalHubToken();
  if (!token) {
    return NextResponse.json({ error: "未登录" }, { status: 401 });
  }

  try {
    const [devices, clients] = await Promise.all([getUserDevices(token), getClients(token)]);
    const clientMap = new Map(clients.map((item) => [item.id, item]));
    const merged = devices.map((device) => ({
      ...clientMap.get(device.id),
      ...device
    }));
    return NextResponse.json({ devices: merged });
  } catch (error) {
    if (error instanceof HubUnauthorizedError) {
      return NextResponse.json({ error: "登录已失效" }, { status: 401 });
    }
    return NextResponse.json({ error: "读取设备列表失败" }, { status: 500 });
  }
}

