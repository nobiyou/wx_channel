"use client";

import { useMemo, useState, useTransition } from "react";

import { formatDateTime, formatTimeAgo } from "@/lib/format";
import type { HubClient } from "@/lib/hub";

type DeviceConsoleProps = {
  initialDevices: HubClient[];
};

type Notice = {
  tone: "success" | "error" | "info";
  text: string;
};

export function DeviceConsole({ initialDevices }: DeviceConsoleProps) {
  const [devices, setDevices] = useState(initialDevices);
  const [bindToken, setBindToken] = useState("");
  const [notice, setNotice] = useState<Notice | null>(null);
  const [busyKey, setBusyKey] = useState("");
  const [pending, startTransition] = useTransition();

  const stats = useMemo(() => {
    const online = devices.filter((item) => item.status === "online").length;
    const locked = devices.filter((item) => item.is_locked).length;
    return {
      total: devices.length,
      online,
      offline: Math.max(devices.length - online, 0),
      locked
    };
  }, [devices]);

  async function refreshDevices() {
    setBusyKey("refresh");
    setNotice({ tone: "info", text: "正在刷新设备列表..." });
    try {
      const response = await fetch("/api/hub/devices", { cache: "no-store" });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(payload.error || "刷新失败");
      }
      setDevices(payload.devices || []);
      setNotice({ tone: "success", text: "设备列表已刷新" });
    } catch (error) {
      setNotice({ tone: "error", text: error instanceof Error ? error.message : "刷新失败" });
    } finally {
      setBusyKey("");
    }
  }

  async function generateBindToken() {
    setBusyKey("bind-token");
    setNotice({ tone: "info", text: "正在生成绑定码..." });
    try {
      const response = await fetch("/api/hub/device/bind-token", {
        method: "POST"
      });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(payload.error || "生成绑定码失败");
      }
      const token = payload.token || payload.data?.token;
      if (!token) {
        throw new Error("Hub 未返回绑定码");
      }
      setBindToken(token);
      setNotice({ tone: "success", text: "绑定码已生成，可在客户端执行绑定命令" });
    } catch (error) {
      setNotice({ tone: "error", text: error instanceof Error ? error.message : "生成绑定码失败" });
    } finally {
      setBusyKey("");
    }
  }

  async function runDeviceAction(
    endpoint: string,
    body: Record<string, unknown>,
    successText: string
  ) {
    const response = await fetch(endpoint, {
      method: "POST",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify(body)
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(payload.error || payload.message || "设备操作失败");
    }
    setNotice({ tone: "success", text: payload.message || successText });
    await refreshDevices();
  }

  function handleRename(device: HubClient) {
    const currentValue = device.display_name || device.hostname || "";
    const nextValue = window.prompt("输入新的设备名称", currentValue);
    if (nextValue === null) {
      return;
    }
    const trimmed = nextValue.trim();
    if (!trimmed) {
      setNotice({ tone: "error", text: "设备名称不能为空" });
      return;
    }

    startTransition(async () => {
      setBusyKey(`rename:${device.id}`);
      try {
        await runDeviceAction(
          "/api/hub/device/rename",
          { device_id: device.id, display_name: trimmed },
          "设备名称已更新"
        );
      } catch (error) {
        setNotice({ tone: "error", text: error instanceof Error ? error.message : "设备名称更新失败" });
      } finally {
        setBusyKey("");
      }
    });
  }

  function handleGroup(device: HubClient) {
    const nextValue = window.prompt("输入设备分组，可留空清除", device.device_group || "");
    if (nextValue === null) {
      return;
    }

    startTransition(async () => {
      setBusyKey(`group:${device.id}`);
      try {
        await runDeviceAction(
          "/api/hub/device/group",
          { device_id: device.id, device_group: nextValue.trim() },
          "设备分组已更新"
        );
      } catch (error) {
        setNotice({ tone: "error", text: error instanceof Error ? error.message : "设备分组更新失败" });
      } finally {
        setBusyKey("");
      }
    });
  }

  function handleLock(device: HubClient) {
    const nextLocked = !device.is_locked;
    startTransition(async () => {
      setBusyKey(`lock:${device.id}`);
      try {
        await runDeviceAction(
          "/api/hub/device/lock",
          { device_id: device.id, is_locked: nextLocked },
          nextLocked ? "设备已锁定" : "设备已解锁"
        );
      } catch (error) {
        setNotice({ tone: "error", text: error instanceof Error ? error.message : "设备锁定状态更新失败" });
      } finally {
        setBusyKey("");
      }
    });
  }

  function handleUnbind(device: HubClient) {
    const ok = window.confirm(`确认解绑设备「${device.display_name || device.hostname || device.id}」吗？`);
    if (!ok) {
      return;
    }
    startTransition(async () => {
      setBusyKey(`unbind:${device.id}`);
      try {
        await runDeviceAction(
          "/api/hub/device/unbind",
          { device_id: device.id },
          "设备已解绑"
        );
      } catch (error) {
        setNotice({ tone: "error", text: error instanceof Error ? error.message : "设备解绑失败" });
      } finally {
        setBusyKey("");
      }
    });
  }

  function handleDelete(device: HubClient) {
    const ok = window.confirm(`确认删除设备「${device.display_name || device.hostname || device.id}」吗？此操作不可撤销。`);
    if (!ok) {
      return;
    }
    startTransition(async () => {
      setBusyKey(`delete:${device.id}`);
      try {
        await runDeviceAction(
          "/api/hub/device/delete",
          { device_id: device.id },
          "设备已删除"
        );
      } catch (error) {
        setNotice({ tone: "error", text: error instanceof Error ? error.message : "设备删除失败" });
      } finally {
        setBusyKey("");
      }
    });
  }

  return (
    <div className="space-y-6">
      <div className="grid gap-4 lg:grid-cols-4">
        <MetricCard label="总设备" value={String(stats.total)} detail="当前账号已绑定设备" />
        <MetricCard label="在线" value={String(stats.online)} detail="status = online" />
        <MetricCard label="离线" value={String(stats.offline)} detail="status != online" />
        <MetricCard label="锁定" value={String(stats.locked)} detail="防止误转移" />
      </div>

      <div className="grid gap-4 xl:grid-cols-[320px_1fr]">
        <section className="rounded-[28px] border border-[var(--border)] bg-[var(--surface-soft)] p-5">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h2 className="m-0 text-xl font-semibold">添加新设备</h2>
              <p className="mt-2 text-sm leading-7 text-[var(--muted)]">
                生成绑定码后，在客户端执行 `client bind &lt;token&gt;` 即可把终端绑定到当前账号。
              </p>
            </div>
            <button
              type="button"
              onClick={generateBindToken}
              disabled={pending || busyKey === "bind-token"}
              className="rounded-full bg-[var(--primary)] px-4 py-2 text-sm text-white transition hover:bg-[var(--primary-strong)] disabled:opacity-60"
            >
              {busyKey === "bind-token" ? "生成中..." : "生成绑定码"}
            </button>
          </div>

          <div className="mt-5 rounded-[24px] border border-dashed border-[var(--border)] bg-white p-4">
            {bindToken ? (
              <>
                <div className="font-mono text-3xl font-semibold tracking-[0.28em] text-[var(--primary)]">{bindToken}</div>
                <div className="mt-3 rounded-2xl bg-[var(--surface-soft)] px-3 py-3 font-mono text-sm text-[var(--text)]">
                  client bind {bindToken}
                </div>
              </>
            ) : (
              <div className="text-sm text-[var(--muted)]">暂未生成绑定码。</div>
            )}
          </div>
        </section>

        <section className="rounded-[28px] border border-[var(--border)] bg-[var(--surface-soft)] p-5">
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <div>
              <h2 className="m-0 text-xl font-semibold">设备列表</h2>
              <p className="mt-2 text-sm leading-7 text-[var(--muted)]">
                当前列表只展示归属于当前账号的设备，但会合并在线状态、能力和页面信息。
              </p>
            </div>
            <button
              type="button"
              onClick={refreshDevices}
              disabled={pending || busyKey === "refresh"}
              className="rounded-full border border-[var(--border)] bg-white px-4 py-2 text-sm text-[var(--text)] transition hover:border-[var(--primary)] hover:text-[var(--primary)] disabled:opacity-60"
            >
              {busyKey === "refresh" ? "刷新中..." : "刷新列表"}
            </button>
          </div>

          {notice ? <NoticeBar notice={notice} /> : null}

          <div className="mt-5 space-y-4">
            {devices.map((device) => {
              const deviceName = device.display_name || device.hostname || "未命名设备";
              const actionBusy = busyKey.endsWith(`:${device.id}`);

              return (
                <article key={device.id} className="rounded-[24px] border border-[var(--border)] bg-white p-4">
                  <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="m-0 text-lg font-semibold">{deviceName}</h3>
                        <StatusPill label={device.status || "unknown"} tone={device.status === "online" ? "good" : "muted"} />
                        {device.is_locked ? <StatusPill label="已锁定" tone="warn" /> : null}
                        {device.device_group ? <StatusPill label={device.device_group} tone="info" /> : null}
                      </div>
                      <div className="mt-2 font-mono text-xs text-[var(--muted)]">{device.id}</div>
                      <div className="mt-4 grid gap-3 text-sm text-[var(--muted)] md:grid-cols-2 xl:grid-cols-3">
                        <div>网络：{device.ip || "未知"}:{device.port || "2025"}</div>
                        <div>版本：v{device.version || "unknown"}</div>
                        <div>页面：{device.page_path || device.href || "未上报"}</div>
                        <div>能力：API {device.api_ready ? "就绪" : "未就绪"}</div>
                        <div>搜索：{device.supports_search ? "可用" : "否"} / 列表：{device.supports_feed ? "可用" : "否"}</div>
                        <div>详情：{device.supports_profile ? "可用" : "否"}</div>
                      </div>
                    </div>

                    <div className="flex flex-col gap-3 lg:w-[260px] lg:items-end">
                      <div className="text-right text-sm text-[var(--muted)]">
                        <div>{formatTimeAgo(device.last_seen)}</div>
                        <div className="mt-1 text-xs">{formatDateTime(device.last_seen)}</div>
                      </div>
                      <div className="flex flex-wrap gap-2 lg:justify-end">
                        <ActionButton label="改名" onClick={() => handleRename(device)} disabled={pending || actionBusy} />
                        <ActionButton label="分组" onClick={() => handleGroup(device)} disabled={pending || actionBusy} />
                        <ActionButton
                          label={device.is_locked ? "解锁" : "锁定"}
                          onClick={() => handleLock(device)}
                          disabled={pending || actionBusy}
                        />
                        <ActionButton label="解绑" onClick={() => handleUnbind(device)} disabled={pending || actionBusy} tone="warn" />
                        <ActionButton label="删除" onClick={() => handleDelete(device)} disabled={pending || actionBusy} tone="danger" />
                      </div>
                    </div>
                  </div>
                </article>
              );
            })}
          </div>

          {devices.length === 0 ? (
            <div className="mt-5 rounded-[24px] border border-dashed border-[var(--border)] px-5 py-10 text-sm text-[var(--muted)]">
              当前账号还没有绑定设备。先生成绑定码，在客户端执行绑定命令。
            </div>
          ) : null}
        </section>
      </div>
    </div>
  );
}

function MetricCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div className="rounded-[26px] border border-[var(--border)] bg-[var(--surface-soft)] p-5">
      <div className="text-xs uppercase tracking-[0.24em] text-[var(--muted)]">{label}</div>
      <div className="mt-4 text-4xl font-semibold">{value}</div>
      <div className="mt-3 text-sm text-[var(--muted)]">{detail}</div>
    </div>
  );
}

function NoticeBar({ notice }: { notice: Notice }) {
  const toneClass =
    notice.tone === "success"
      ? "border-[var(--primary)]/25 bg-[var(--primary)]/8 text-[var(--primary)]"
      : notice.tone === "error"
        ? "border-[var(--danger)]/25 bg-[var(--danger)]/8 text-[var(--danger)]"
        : "border-[var(--accent)]/30 bg-[var(--accent)]/12 text-[var(--text)]";

  return <div className={`mt-5 rounded-2xl border px-4 py-3 text-sm ${toneClass}`}>{notice.text}</div>;
}

function ActionButton({
  label,
  onClick,
  disabled,
  tone = "default"
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  tone?: "default" | "warn" | "danger";
}) {
  const className =
    tone === "danger"
      ? "border-[var(--danger)]/25 text-[var(--danger)] hover:bg-[var(--danger)]/8"
      : tone === "warn"
        ? "border-[var(--accent)]/30 text-[var(--text)] hover:bg-[var(--accent)]/12"
        : "border-[var(--border)] text-[var(--text)] hover:border-[var(--primary)] hover:text-[var(--primary)]";

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={`rounded-full border bg-white px-4 py-2 text-sm transition disabled:cursor-not-allowed disabled:opacity-60 ${className}`}
    >
      {label}
    </button>
  );
}

function StatusPill({
  label,
  tone
}: {
  label: string;
  tone: "good" | "muted" | "warn" | "info";
}) {
  const className =
    tone === "good"
      ? "bg-[var(--primary)]/10 text-[var(--primary)]"
      : tone === "warn"
        ? "bg-[var(--accent)]/16 text-[var(--text)]"
        : tone === "info"
          ? "bg-sky-100 text-sky-700"
          : "bg-[var(--surface-soft)] text-[var(--muted)]";

  return <span className={`rounded-full px-3 py-1 text-xs ${className}`}>{label}</span>;
}

