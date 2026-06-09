import Link from "next/link";

import { HubConfigError, getHubApiBase } from "@/lib/hub";

const navCards = [
  {
    title: "登录入口",
    href: "/login",
    description: "使用 Hub 账号登录，进入设备与任务控制台。"
  },
  {
    title: "仪表盘",
    href: "/dashboard",
    description: "查看在线终端、任务统计、订阅规模与系统指标概览。"
  },
  {
    title: "设备管理",
    href: "/devices",
    description: "浏览用户设备、在线状态、分组和终端能力。"
  },
  {
    title: "任务中心",
    href: "/tasks",
    description: "检查远程调用和订阅拉取任务的执行结果。"
  },
  {
    title: "订阅管理",
    href: "/subscriptions",
    description: "查看视频号订阅、更新状态和新增视频规模。"
  }
];

export default function Home() {
  let apiBase = "";
  let apiConfigured = true;

  try {
    apiBase = getHubApiBase();
  } catch (error) {
    if (error instanceof HubConfigError) {
      apiConfigured = false;
    } else {
      throw error;
    }
  }

  return (
    <main className="min-h-screen px-6 py-10 text-foreground lg:px-10">
      <section className="mx-auto flex max-w-7xl flex-col gap-10">
        <div className="rounded-[32px] border border-[var(--border)] bg-[var(--surface)]/92 p-8 shadow-[0_24px_80px_rgba(20,38,18,0.08)] backdrop-blur">
          <div className="mb-8 flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div className="max-w-3xl">
              <div className="mb-3 inline-flex rounded-full border border-[var(--border)] bg-[var(--surface-soft)] px-4 py-1 text-sm text-[var(--muted)]">
                Sites 版 Hub 控制台
              </div>
              <h1 className="m-0 text-4xl font-semibold tracking-tight lg:text-6xl">
                把 Hub 前端抽成独立站点，而不是继续绑在本地可执行程序里。
              </h1>
              <p className="mt-5 max-w-2xl text-base leading-8 text-[var(--muted)] lg:text-lg">
                这套前端面向 Linux / Hub 场景，负责登录、终端状态、任务追踪和订阅管理。
                下载、注入、解密和本地文件能力仍由现有 Go Hub 后端负责。
              </p>
            </div>
            <div className="rounded-[28px] border border-[var(--border)] bg-[var(--surface-soft)] p-5 text-sm text-[var(--muted)] lg:w-[320px]">
              <div className="mb-3 text-xs uppercase tracking-[0.24em] text-[var(--primary)]">Status</div>
            <div className="mb-2 text-2xl font-semibold text-[var(--text)]">Ready For Wiring</div>
              <div>
                {apiConfigured
                  ? `当前 Hub API: ${apiBase}`
                  : "下一步先把可访问的 Hub API 地址写入站点环境变量。"}
              </div>
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
            {navCards.map((card) => (
              <Link
                key={card.href}
                href={card.href}
                className="group rounded-[26px] border border-[var(--border)] bg-[var(--surface)] p-5 transition hover:-translate-y-1 hover:border-[var(--primary)] hover:shadow-[0_18px_40px_rgba(31,106,59,0.12)]"
              >
                <div className="mb-3 text-sm font-medium text-[var(--primary)]">{card.title}</div>
                <div className="text-sm leading-7 text-[var(--muted)]">{card.description}</div>
                <div className="mt-5 text-sm font-medium text-[var(--text)] group-hover:text-[var(--primary)]">
                  进入页面 →
                </div>
              </Link>
            ))}
          </div>
        </div>
      </section>
    </main>
  );
}
