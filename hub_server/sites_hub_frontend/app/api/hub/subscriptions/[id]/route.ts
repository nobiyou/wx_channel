import { NextRequest } from "next/server";

import { proxyHubPassthrough } from "@/app/api/hub/_utils";

export async function DELETE(
  request: NextRequest,
  context: { params: Promise<{ id: string }> }
) {
  const { id } = await context.params;
  return proxyHubPassthrough(request, `/api/subscriptions/${id}`, "DELETE");
}

