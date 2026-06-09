import { NextRequest } from "next/server";

import { proxyHubPassthrough } from "@/app/api/hub/_utils";

export async function POST(request: NextRequest) {
  return proxyHubPassthrough(request, "/api/device/delete", "POST");
}

