import { AppShell } from "@/components/shell";
import { formatDateTime, formatTimeAgo } from "@/lib/format";
import { HubUnauthorizedError, getTasks } from "@/lib/hub";
import { requireHubToken } from "@/lib/session";
import { redirect } from "next/navigation";

export default async function TasksPage() {
  const token = await requireHubToken();
  const tasks = await getTasks(token).catch((error) => {
    if (error instanceof HubUnauthorizedError) {
      redirect("/login");
    }
    return [];
  });

  return (
    <AppShell
      title="任务中心"
      subtitle="先把 Hub 后端已有的任务历史接到站点里，后续再考虑详情弹窗、筛选和自动刷新。"
    >
      <div className="grid gap-4 lg:grid-cols-3">
        <TaskMetric title="全部任务" value={String(tasks.length)} />
        <TaskMetric title="成功 / 完成" value={String(tasks.filter((item) => item.status === "success" || item.status === "done").length)} />
        <TaskMetric title="失败 / 超时" value={String(tasks.filter((item) => item.status === "failed" || item.status === "timeout").length)} />
      </div>

      <div className="mt-8 space-y-4">
        {tasks.map((task) => (
          <article key={task.id} className="rounded-[26px] border border-[var(--border)] bg-[var(--surface-soft)] p-5">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div>
                <div className="text-lg font-medium">#{task.id} · {task.type || "unknown"}</div>
                <div className="mt-2 text-sm text-[var(--muted)]">{task.node_id || "未关联终端"}</div>
              </div>
              <div className="text-sm text-[var(--muted)]">
                <div>{task.status || "unknown"}</div>
                <div className="mt-2">{formatTimeAgo(task.created_at)}</div>
              </div>
            </div>
            <div className="mt-4 text-xs text-[var(--muted)]">{formatDateTime(task.created_at)}</div>
          </article>
        ))}

        {tasks.length === 0 ? (
          <div className="rounded-[26px] border border-dashed border-[var(--border)] px-5 py-10 text-sm text-[var(--muted)]">
            当前没有读到任务记录。
          </div>
        ) : null}
      </div>
    </AppShell>
  );
}

function TaskMetric({ title, value }: { title: string; value: string }) {
  return (
    <div className="rounded-[24px] border border-[var(--border)] bg-[var(--surface-soft)] p-5">
      <div className="text-xs uppercase tracking-[0.22em] text-[var(--muted)]">{title}</div>
      <div className="mt-3 text-3xl font-semibold">{value}</div>
    </div>
  );
}
