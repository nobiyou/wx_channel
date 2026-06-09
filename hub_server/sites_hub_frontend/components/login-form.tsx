"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

export function LoginForm({ disabled = false }: { disabled?: boolean }) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (disabled) {
      setError("当前未配置 HUB_API_BASE，暂时无法登录。");
      return;
    }

    setSubmitting(true);
    setError("");

    try {
      const response = await fetch("/api/session/login", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify({ email, password })
      });

      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(payload.error || "登录失败");
      }

      router.push("/dashboard");
      router.refresh();
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : "登录失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="mt-8 space-y-5">
      <div>
        <label className="mb-2 block text-sm font-medium text-[var(--text)]">邮箱</label>
        <input
          value={email}
          onChange={(event) => setEmail(event.target.value)}
        type="email"
        placeholder="name@example.com"
        disabled={disabled}
        className="w-full rounded-2xl border border-[var(--border)] bg-white px-4 py-3 text-[var(--text)] outline-none transition focus:border-[var(--primary)]"
        required
      />
      </div>

      <div>
        <label className="mb-2 block text-sm font-medium text-[var(--text)]">密码</label>
        <input
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        type="password"
        placeholder="请输入密码"
        disabled={disabled}
        className="w-full rounded-2xl border border-[var(--border)] bg-white px-4 py-3 text-[var(--text)] outline-none transition focus:border-[var(--primary)]"
        required
      />
      </div>

      {error ? (
        <div className="rounded-2xl border border-[var(--danger)]/30 bg-[var(--danger)]/8 px-4 py-3 text-sm text-[var(--danger)]">
          {error}
        </div>
      ) : disabled ? (
        <div className="rounded-2xl border border-[var(--accent)]/35 bg-[var(--accent)]/10 px-4 py-3 text-sm text-[var(--text)]">
          先为站点配置 `HUB_API_BASE`，再登录 Hub。
        </div>
      ) : null}

      <button
        type="submit"
        disabled={submitting || disabled}
        className="w-full rounded-2xl bg-[var(--primary)] px-4 py-3 text-sm font-medium text-white transition hover:bg-[var(--primary-strong)] disabled:cursor-not-allowed disabled:opacity-60"
      >
        {submitting ? "登录中..." : "登录 Hub"}
      </button>
    </form>
  );
}
