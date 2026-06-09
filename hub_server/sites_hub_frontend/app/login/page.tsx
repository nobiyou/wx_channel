import { redirect } from "next/navigation";

import { LoginForm } from "@/components/login-form";
import { HubConfigError, getHubApiBase } from "@/lib/hub";
import { getOptionalHubToken } from "@/lib/session";

export default async function LoginPage() {
  const token = await getOptionalHubToken();
  if (token) {
    redirect("/dashboard");
  }

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
    <main className="flex min-h-screen items-center justify-center px-4 py-10">
      <div className="grid w-full max-w-5xl gap-6 lg:grid-cols-[1.15fr_0.85fr]">
        <section className="rounded-[32px] border border-[var(--border)] bg-[var(--surface)] p-8 shadow-[0_24px_80px_rgba(20,38,18,0.08)] lg:p-10">
          <div className="mb-4 inline-flex rounded-full border border-[var(--border)] bg-[var(--surface-soft)] px-4 py-1 text-sm text-[var(--muted)]">
            Hub 登录入口
          </div>
          <h1 className="m-0 text-4xl font-semibold tracking-tight">站点侧会代 Hub 完成登录交换，但认证仍由 Hub API 负责。</h1>
          <p className="mt-5 max-w-2xl text-base leading-8 text-[var(--muted)]">
            当前版本已经通过站点自己的会话路由提交邮箱密码，向 Hub 换取 JWT，
            然后写入 HttpOnly cookie。后续只需要继续补齐统一代理层和写操作页面，
            不需要再把登录流程搬回本地程序。
          </p>

          <LoginForm disabled={!apiConfigured} />

          <div className="mt-8 rounded-[28px] border border-[var(--border)] bg-[var(--surface-soft)] p-6">
            <div className="mb-3 text-sm font-medium text-[var(--primary)]">当前接入目标</div>
            <div className="font-mono text-sm text-[var(--text)]">{apiConfigured ? apiBase : "未配置 HUB_API_BASE"}</div>
            <div className="mt-4 text-sm leading-7 text-[var(--muted)]">
              当前版本已经通过 Sites 自身的登录路由换取 Hub JWT，并写入 HttpOnly cookie。
              当前依赖的核心接口是 `POST /api/auth/login` 与 `GET /api/auth/profile`。
            </div>
          </div>
        </section>

        <section className="rounded-[32px] border border-[var(--border)] bg-[var(--primary-strong)] p-8 text-white shadow-[0_24px_80px_rgba(12,45,25,0.22)]">
          <div className="mb-3 text-xs uppercase tracking-[0.28em] text-white/70">Next Step</div>
          <h2 className="m-0 text-2xl font-semibold">建议接法</h2>
          <ul className="mt-5 list-none space-y-4 p-0 text-sm leading-7 text-white/84">
            <li>1. 已通过站点登录路由将 Hub JWT 存成 HttpOnly cookie。</li>
            <li>2. 仪表盘、设备、任务、订阅页已优先改为服务端读取受保护 API。</li>
            <li>3. 下一步补 `/api/hub/*` 统一代理，可把后续写操作也纳入同一层。</li>
            <li>4. 解绑设备、更新订阅、搜索等交互页可继续逐页迁移。</li>
          </ul>
        </section>
      </div>
    </main>
  );
}
