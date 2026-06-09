import { NextResponse } from "next/server";

import { HUB_TOKEN_COOKIE } from "@/lib/hub";

export async function POST() {
  const response = NextResponse.json({ ok: true });
  response.cookies.set(HUB_TOKEN_COOKIE, "", {
    httpOnly: true,
    sameSite: "lax",
    secure: false,
    path: "/",
    maxAge: 0
  });
  return response;
}
