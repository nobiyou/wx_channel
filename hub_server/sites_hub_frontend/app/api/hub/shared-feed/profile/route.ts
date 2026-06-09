import { NextRequest } from "next/server";

import { proxyHubPassthrough } from "@/app/api/hub/_utils";

export async function GET(request: NextRequest) {
  const query = request.nextUrl.search;
  return proxyHubPassthrough(request, `/api/channels/shared_feed/profile${query}`, "GET");
}

export async function POST(request: NextRequest) {
  return proxyHubPassthrough(request, "/api/channels/shared_feed/profile", "POST");
}
