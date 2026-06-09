import { AppShell } from "@/components/shell";
import { formatTimeAgo } from "@/lib/format";
import { HubUnauthorizedError, getDashboardData } from "@/lib/hub";
import { requireHubToken } from "@/lib/session";
import { redirect } from "next/navigation";

export default async function DashboardPage() {
  const token = await requireHubToken();
  const data = await getDashboardData(token).catch((error) => {
    if (error instanceof HubUnauthorizedError) {
      redirect("/login");
    }
    return {
      clients: [],
      tasks: [],
      subscriptions: []
    };
  });

  const onlineClients = data.clients.filter((item) => item.status === "online");
  const searchReady = onlineClients.filter((item) => item.supports_search).length;
  const doneTasks = data.tasks.filter((item) => item.status === "success" || item.status === "done").length;

  return (
    <AppShell
      title="Hub 仪表盘"
      subtitle="面向 Linux / Hub 的 Sites 版运营面板。这里先接入 Hub 的只读核心数据，验证前后端分层。"
    >
      <div className="grid gap-4 lg:grid-cols-4">
        <MetricCard label="在线终端" value={String(onlineClients.length)} detail="来自 /api/clients" />
        <MetricCard label="搜索就绪" value={String(searchReady)} detail="supports_search" />
        <MetricCard label="近期任务" value={String(data.tasks.length)} detail={`${doneTasks} 条已完成`} />
        <MetricCard label="订阅数" value={String(data.subscriptions.length)} detail="来自 /api/subscriptions" />
      </div>

      <div className="mt-8 grid gap-6 xl:grid-cols-[1.15fr_0.85fr]">
        <section className="rounded-[28px] border border-[var(--border)] bg-[var(--surface-soft)] p-5">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="m-0 text-xl font-semibold">在线终端</h2>
            <span className="text-sm text-[var(--muted)]">前 6 台</span>
          </div>
          <div className="space-y-3">
            {data.clients.slice(0, 6).map((client) => (
              <div
                key={client.id}
                className="flex items-start justify-between rounded-2xl border border-[var(--border)] bg-white px-4 py-4"
              >
                <div>
                  <div className="font-medium">{client.hostname || client.id}</div>
                  <div className="mt-1 text-xs text-[var(--muted)]">{client.id}</div>
                  <div className="mt-2 text-sm text-[var(--muted)]">
                    {client.page_path || client.href || "未上报页面"}
                  </div>
                </div>
                <div className="text-right">
                  <div className="rounded-full bg-[var(--surface-soft)] px-3 py-1 text-xs text-[var(--primary)]">
                    {client.status || "unknown"}
                  </div>
                  <div className="mt-3 text-xs text-[var(--muted)]">{formatTimeAgo(client.last_seen)}</div>
                </div>
              </div>
            ))}
            {data.clients.length === 0 ? <EmptyHint text="当前没有读到 Hub 终端数据。" /> : null}
          </div>
        </section>

        <section className="rounded-[28px] border border-[var(--border)] bg-[var(--surface-soft)] p-5">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="m-0 text-xl font-semibold">最近任务</h2>
            <span className="text-sm text-[var(--muted)]">前 8 条</span>
          </div>
          <div className="space-y-3">
            {data.tasks.slice(0, 8).map((task) => (
              <div key={task.id} className="rounded-2xl border border-[var(--border)] bg-white px-4 py-4">
                <div className="flex items-center justify-between">
                  <div className="font-medium">#{task.id} {task.type || "unknown"}</div>
                  <div className="text-xs text-[var(--muted)]">{task.status || "unknown"}</div>
                </div>
                <div className="mt-2 text-xs text-[var(--muted)]">{task.node_id || "未关联终端"}</div>
                <div className="mt-3 text-sm text-[var(--muted)]">{formatTimeAgo(task.created_at)}</div>
              </div>
            ))}
            {data.tasks.length === 0 ? <EmptyHint text="当前没有读到任务数据。" /> : null}
          </div>
        </section>
      </div>
    </AppShell>
  );
}

function MetricCard({
  label,
  value,
  detail
}: {
  label: string;
  value: string;
  detail: string;
}) {
  return (
    <div className="rounded-[26px] border border-[var(--border)] bg-[var(--surface-soft)] p-5">
      <div className="text-xs uppercase tracking-[0.24em] text-[var(--muted)]">{label}</div>
      <div className="mt-4 text-4xl font-semibold">{value}</div>
      <div className="mt-3 text-sm text-[var(--muted)]">{detail}</div>
    </div>
  );
}

function EmptyHint({ text }: { text: string }) {
  return <div className="rounded-2xl border border-dashed border-[var(--border)] px-4 py-6 text-sm text-[var(--muted)]">{text}</div>;
}
