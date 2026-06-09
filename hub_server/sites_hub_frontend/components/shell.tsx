import Link from "next/link";
import { ReactNode } from "react";
import { headers } from "next/headers";

import { formatDateTime } from "@/lib/format";
import { getProfile } from "@/lib/hub";
import { getOptionalHubToken } from "@/lib/session";
import { LogoutButton } from "@/components/logout-button";

const navItems = [
  { href: "/dashboard", label: "仪表盘" },
  { href: "/devices", label: "设备" },
  { href: "/tasks", label: "任务" },
  { href: "/subscriptions", label: "订阅" }
];

export async function AppShell({
  title,
  subtitle,
  children
}: {
  title: string;
  subtitle: string;
  children: ReactNode;
}) {
  const token = await getOptionalHubToken();
  const user = token ? await getProfile(token).catch(() => null) : null;
  const headerStore = await headers();
  const pathname = headerStore.get("x-invoke-path") || "";

  return (
    <div className="min-h-screen bg-background">
      <div className="mx-auto grid min-h-screen max-w-7xl gap-6 px-4 py-4 lg:grid-cols-[280px_1fr] lg:px-6">
        <aside className="rounded-[28px] border border-[var(--border)] bg-[var(--surface)] p-5 shadow-[0_18px_55px_rgba(22,34,22,0.08)]">
          <div className="mb-8">
            <div className="mb-3 inline-flex h-12 w-12 items-center justify-center rounded-2xl bg-[var(--primary)] text-xl font-semibold text-white">
              H
            </div>
            <div className="text-2xl font-semibold">Hub Control</div>
            <div className="mt-2 text-sm leading-7 text-[var(--muted)]">
              Sites 版控制台，只承载前端视图，不替代 Hub 后端能力。
            </div>
          </div>

          <nav className="space-y-2">
            {navItems.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                className={`block rounded-2xl border px-4 py-3 text-sm font-medium transition ${
                  pathname.startsWith(item.href)
                    ? "border-[var(--primary)]/20 bg-white text-[var(--primary)]"
                    : "border-transparent bg-[var(--surface-soft)] text-[var(--text)] hover:border-[var(--border)] hover:bg-white"
                }`}
              >
                {item.label}
              </Link>
            ))}
          </nav>

          <div className="mt-10 rounded-2xl border border-[var(--border)] bg-[var(--surface-soft)] p-4 text-sm text-[var(--muted)]">
            <div className="mb-2 text-xs uppercase tracking-[0.24em] text-[var(--primary)]">Deploy Note</div>
            <div>
              站点部署后，需要通过环境变量提供 Hub API 地址，并保证 Hub 侧开放可访问的认证接口。
            </div>
          </div>
        </aside>

        <main className="rounded-[32px] border border-[var(--border)] bg-[var(--surface)] p-6 shadow-[0_20px_60px_rgba(20,38,18,0.08)] lg:p-8">
          <header className="mb-8 flex flex-col gap-3 border-b border-[var(--border)] pb-6 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <div className="text-xs uppercase tracking-[0.26em] text-[var(--primary)]">Hub Sites Frontend</div>
              <h1 className="m-0 mt-3 text-3xl font-semibold tracking-tight">{title}</h1>
              <p className="mt-3 max-w-2xl text-sm leading-7 text-[var(--muted)]">{subtitle}</p>
            </div>
            <div className="flex flex-col gap-3 lg:items-end">
              <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface-soft)] px-4 py-3 text-sm text-[var(--muted)]">
                <div className="font-medium text-[var(--text)]">{user?.email || "未登录"}</div>
                <div className="mt-1">
                  {user?.role || "anonymous"} · {formatDateTime(new Date().toISOString())}
                </div>
              </div>
              <LogoutButton />
            </div>
          </header>

          {children}
        </main>
      </div>
    </div>
  );
}
