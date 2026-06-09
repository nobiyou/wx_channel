"use client";

import Image from "next/image";
import { useMemo, useState, useTransition } from "react";

import { formatDateTime, formatTimeAgo } from "@/lib/format";
import type {
  HubSharedFeedCompatResponse,
  HubSharedFeedProfile,
  HubSubscription,
  HubVideo
} from "@/lib/hub";

type SubscriptionConsoleProps = {
  initialSubscriptions: HubSubscription[];
};

type Notice = {
  tone: "success" | "error" | "info";
  text: string;
};

type VideoPanelState = {
  subscriptionId: number;
  page: number;
  total: number;
  videos: HubVideo[];
  title: string;
};

const EMPTY_SHARE_PROFILE: HubSharedFeedProfile = {};

export function SubscriptionConsole({ initialSubscriptions }: SubscriptionConsoleProps) {
  const [subscriptions, setSubscriptions] = useState(initialSubscriptions);
  const [notice, setNotice] = useState<Notice | null>(null);
  const [busyKey, setBusyKey] = useState("");
  const [videoPanel, setVideoPanel] = useState<VideoPanelState | null>(null);
  const [shareUrl, setShareUrl] = useState("");
  const [manualUsername, setManualUsername] = useState("");
  const [manualNickname, setManualNickname] = useState("");
  const [manualSignature, setManualSignature] = useState("");
  const [manualHeadUrl, setManualHeadUrl] = useState("");
  const [resolvedProfile, setResolvedProfile] = useState<HubSharedFeedProfile>(EMPTY_SHARE_PROFILE);
  const [pending, startTransition] = useTransition();

  const totalVideos = useMemo(
    () => subscriptions.reduce((sum, item) => sum + (item.video_count || 0), 0),
    [subscriptions]
  );

  async function refreshSubscriptions() {
    setBusyKey("refresh-subs");
    setNotice({ tone: "info", text: "正在刷新订阅列表..." });
    try {
      const response = await fetch("/api/hub/subscriptions", { cache: "no-store" });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(payload.error || "刷新失败");
      }
      setSubscriptions(payload.subscriptions || []);
      setNotice({ tone: "success", text: "订阅列表已刷新" });
    } catch (error) {
      setNotice({ tone: "error", text: error instanceof Error ? error.message : "刷新失败" });
    } finally {
      setBusyKey("");
    }
  }

  async function fetchSubscription(id: number) {
    const response = await fetch(`/api/hub/subscriptions/${id}/fetch`, {
      method: "POST"
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(payload.error || payload.message || "更新订阅失败");
    }
    return payload;
  }

  async function deleteSubscription(id: number) {
    const response = await fetch(`/api/hub/subscriptions/${id}`, {
      method: "DELETE"
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(payload.error || payload.message || "删除订阅失败");
    }
    return payload;
  }

  async function createSubscription(payload: {
    wx_username: string;
    wx_nickname?: string;
    wx_signature?: string;
    wx_head_url?: string;
  }) {
    const response = await fetch("/api/hub/subscriptions", {
      method: "POST",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify(payload)
    });
    const body = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(body.error || body.message || "新增订阅失败");
    }
    return body;
  }

  async function resolveSharedFeedProfile(url: string) {
    const response = await fetch(`/api/hub/shared-feed/profile?url=${encodeURIComponent(url)}`, {
      cache: "no-store"
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(payload.error || payload.message || "解析分享链接失败");
    }
    return buildProfileFromCompat(payload);
  }

  async function loadVideos(subscription: HubSubscription, page = 1) {
    setBusyKey(`videos:${subscription.id}:${page}`);
    try {
      const response = await fetch(`/api/hub/subscriptions/${subscription.id}/videos?page=${page}`, {
        cache: "no-store"
      });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(payload.error || "读取订阅视频失败");
      }
      const data = payload.data || {};
      setVideoPanel({
        subscriptionId: subscription.id,
        page: data.page || page,
        total: data.total || 0,
        videos: data.videos || [],
        title: subscription.wx_nickname || subscription.wx_username || `订阅 #${subscription.id}`
      });
    } catch (error) {
      setNotice({ tone: "error", text: error instanceof Error ? error.message : "读取订阅视频失败" });
    } finally {
      setBusyKey("");
    }
  }

  function resetResolvedProfile() {
    setResolvedProfile(EMPTY_SHARE_PROFILE);
  }

  function handleResolveShare() {
    const url = shareUrl.trim();
    if (!url) {
      setNotice({ tone: "error", text: "请先粘贴视频号分享链接" });
      return;
    }

    startTransition(async () => {
      setBusyKey("resolve-share");
      setNotice({ tone: "info", text: "正在解析分享链接..." });
      try {
        const profile = await resolveSharedFeedProfile(url);
        setResolvedProfile(profile);
        setNotice({ tone: "success", text: "已识别分享链接中的作者信息，可以直接订阅" });
      } catch (error) {
        setResolvedProfile(EMPTY_SHARE_PROFILE);
        setNotice({ tone: "error", text: error instanceof Error ? error.message : "解析分享链接失败" });
      } finally {
        setBusyKey("");
      }
    });
  }

  function handleCreateManual() {
    const wxUsername = manualUsername.trim();
    if (!wxUsername) {
      setNotice({ tone: "error", text: "finderUsername 不能为空" });
      return;
    }

    startTransition(async () => {
      setBusyKey("create-manual");
      setNotice({ tone: "info", text: `正在订阅 ${wxUsername} ...` });
      try {
        await createSubscription({
          wx_username: wxUsername,
          wx_nickname: manualNickname.trim(),
          wx_signature: manualSignature.trim(),
          wx_head_url: manualHeadUrl.trim()
        });
        setManualUsername("");
        setManualNickname("");
        setManualSignature("");
        setManualHeadUrl("");
        await refreshSubscriptions();
        setNotice({ tone: "success", text: `已订阅 ${wxUsername}` });
      } catch (error) {
        setNotice({ tone: "error", text: error instanceof Error ? error.message : "新增订阅失败" });
      } finally {
        setBusyKey("");
      }
    });
  }

  function handleCreateFromShare() {
    const wxUsername = resolvedProfile.wx_username?.trim();
    if (!wxUsername) {
      setNotice({ tone: "error", text: "当前解析结果缺少作者 username，无法创建订阅" });
      return;
    }

    startTransition(async () => {
      setBusyKey("create-share");
      setNotice({ tone: "info", text: `正在订阅 ${resolvedProfile.wx_nickname || wxUsername} ...` });
      try {
        await createSubscription({
          wx_username: wxUsername,
          wx_nickname: resolvedProfile.wx_nickname?.trim(),
          wx_signature: resolvedProfile.wx_signature?.trim(),
          wx_head_url: resolvedProfile.wx_head_url?.trim()
        });
        await refreshSubscriptions();
        setNotice({ tone: "success", text: `已订阅 ${resolvedProfile.wx_nickname || wxUsername}` });
      } catch (error) {
        setNotice({ tone: "error", text: error instanceof Error ? error.message : "新增订阅失败" });
      } finally {
        setBusyKey("");
      }
    });
  }

  function handleFetchOne(subscription: HubSubscription) {
    startTransition(async () => {
      setBusyKey(`fetch:${subscription.id}`);
      setNotice({ tone: "info", text: `正在更新「${subscription.wx_nickname || subscription.wx_username || subscription.id}」...` });
      try {
        const payload = await fetchSubscription(subscription.id);
        const data = payload.data || {};
        setNotice({
          tone: "success",
          text: `更新完成，新抓取 ${data.new_videos ?? 0} 条，总计 ${data.total_videos ?? subscription.video_count ?? 0} 条`
        });
        await refreshSubscriptions();
      } catch (error) {
        setNotice({ tone: "error", text: error instanceof Error ? error.message : "更新订阅失败" });
      } finally {
        setBusyKey("");
      }
    });
  }

  function handleFetchAll() {
    if (subscriptions.length === 0) {
      return;
    }
    startTransition(async () => {
      setBusyKey("fetch-all");
      setNotice({ tone: "info", text: "正在依次更新全部订阅，这可能需要一点时间..." });
      let successCount = 0;
      let failureCount = 0;
      for (const subscription of subscriptions) {
        try {
          await fetchSubscription(subscription.id);
          successCount += 1;
        } catch {
          failureCount += 1;
        }
      }
      await refreshSubscriptions();
      setNotice({
        tone: failureCount > 0 ? "info" : "success",
        text: `全部更新完成，成功 ${successCount} 个，失败 ${failureCount} 个`
      });
      setBusyKey("");
    });
  }

  function handleDelete(subscription: HubSubscription) {
    const label = subscription.wx_nickname || subscription.wx_username || `订阅 #${subscription.id}`;
    const ok = window.confirm(`确认删除「${label}」吗？关联视频也会一起清理。`);
    if (!ok) {
      return;
    }
    startTransition(async () => {
      setBusyKey(`delete:${subscription.id}`);
      try {
        await deleteSubscription(subscription.id);
        setNotice({ tone: "success", text: `已删除 ${label}` });
        setSubscriptions((current) => current.filter((item) => item.id !== subscription.id));
        if (videoPanel?.subscriptionId === subscription.id) {
          setVideoPanel(null);
        }
      } catch (error) {
        setNotice({ tone: "error", text: error instanceof Error ? error.message : "删除订阅失败" });
      } finally {
        setBusyKey("");
      }
    });
  }

  async function copyText(value: string, successText: string) {
    try {
      await navigator.clipboard.writeText(value);
      setNotice({ tone: "success", text: successText });
    } catch {
      setNotice({ tone: "error", text: "复制失败，请检查浏览器权限" });
    }
  }

  function handleOpenVideo(video: HubVideo) {
    const playURL = buildPlayableVideoURL(video);
    if (!playURL) {
      setNotice({ tone: "error", text: "当前视频缺少可播放地址" });
      return;
    }
    window.open(playURL, "_blank", "noopener,noreferrer");
  }

  function handleCopyVideo(video: HubVideo) {
    const playURL = buildPlayableVideoURL(video);
    if (!playURL) {
      setNotice({ tone: "error", text: "当前视频缺少可复制地址" });
      return;
    }
    void copyText(playURL, "播放链接已复制");
  }

  function handleDownloadVideo(video: HubVideo) {
    const playURL = buildPlayableVideoURL(video);
    if (!playURL) {
      setNotice({ tone: "error", text: "当前视频缺少下载地址" });
      return;
    }
    window.open(playURL, "_blank", "noopener,noreferrer");
  }

  return (
    <div className="space-y-6">
      <div className="grid gap-4 lg:grid-cols-3">
        <MetricCard label="订阅账号" value={String(subscriptions.length)} detail="当前账号已订阅作者" />
        <MetricCard label="视频总量" value={String(totalVideos)} detail="本地库中的已抓取视频" />
        <MetricCard
          label="最近更新"
          value={subscriptions[0]?.last_fetched_at ? formatTimeAgo(subscriptions[0].last_fetched_at) : "暂无"}
          detail="按创建时间倒序"
        />
      </div>

      <section className="grid gap-4 xl:grid-cols-[1.1fr_0.9fr]">
        <article className="rounded-[28px] border border-[var(--border)] bg-[var(--surface-soft)] p-5">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h2 className="m-0 text-xl font-semibold">新增订阅</h2>
              <p className="mt-2 text-sm leading-7 text-[var(--muted)]">
                支持两种入口：直接填写作者信息，或粘贴视频号分享链接自动识别作者后订阅。
              </p>
            </div>
          </div>

          <div className="mt-5 grid gap-4 lg:grid-cols-2">
            <div className="rounded-[24px] border border-[var(--border)] bg-white p-4">
              <div className="text-sm font-medium">手动添加作者</div>
              <div className="mt-4 grid gap-3">
                <FieldInput
                  label="finderUsername"
                  value={manualUsername}
                  onChange={setManualUsername}
                  placeholder="gh_xxxxxxxx"
                />
                <FieldInput
                  label="昵称"
                  value={manualNickname}
                  onChange={setManualNickname}
                  placeholder="作者昵称"
                />
                <FieldInput
                  label="签名"
                  value={manualSignature}
                  onChange={setManualSignature}
                  placeholder="作者签名，可留空"
                />
                <FieldInput
                  label="头像 URL"
                  value={manualHeadUrl}
                  onChange={setManualHeadUrl}
                  placeholder="https://..."
                />
                <div className="pt-1">
                  <ToolbarButton
                    label={busyKey === "create-manual" ? "提交中..." : "创建订阅"}
                    onClick={handleCreateManual}
                    disabled={pending || busyKey === "create-manual"}
                    primary
                  />
                </div>
              </div>
            </div>

            <div className="rounded-[24px] border border-[var(--border)] bg-white p-4">
              <div className="text-sm font-medium">分享链接识别</div>
              <div className="mt-4 grid gap-3">
                <FieldTextarea
                  label="视频号分享链接"
                  value={shareUrl}
                  onChange={setShareUrl}
                  placeholder="https://weixin.qq.com/sph/..."
                />
                <div className="flex flex-wrap gap-2">
                  <ToolbarButton
                    label={busyKey === "resolve-share" ? "识别中..." : "识别作者"}
                    onClick={handleResolveShare}
                    disabled={pending || busyKey === "resolve-share"}
                    primary
                  />
                  <ToolbarButton
                    label="清空"
                    onClick={() => {
                      setShareUrl("");
                      resetResolvedProfile();
                    }}
                    disabled={pending}
                  />
                </div>
              </div>

              {hasResolvedProfile(resolvedProfile) ? (
                <div className="mt-4 rounded-[20px] border border-[var(--border)] bg-[var(--surface-soft)] p-4">
                  <div className="flex items-start gap-3">
                    {resolvedProfile.wx_head_url ? (
                      <div className="relative h-14 w-14 overflow-hidden rounded-2xl border border-[var(--border)]">
                        <Image src={resolvedProfile.wx_head_url} alt={resolvedProfile.wx_nickname || "分享作者"} fill sizes="56px" className="object-cover" unoptimized />
                      </div>
                    ) : (
                      <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-white text-lg font-semibold text-[var(--primary)]">
                        {(resolvedProfile.wx_nickname || "?").slice(0, 1)}
                      </div>
                    )}
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-base font-medium">{resolvedProfile.wx_nickname || "未识别昵称"}</div>
                      <div className="mt-1 truncate font-mono text-xs text-[var(--muted)]">
                        {resolvedProfile.wx_username || "缺少 username"}
                      </div>
                      <div className="mt-2 text-sm leading-7 text-[var(--muted)]">
                        {resolvedProfile.wx_signature || resolvedProfile.description || "暂无简介"}
                      </div>
                    </div>
                  </div>
                  <div className="mt-4 flex flex-wrap gap-2">
                    <ToolbarButton
                      label={busyKey === "create-share" ? "订阅中..." : "订阅该作者"}
                      onClick={handleCreateFromShare}
                      disabled={pending || busyKey === "create-share"}
                      primary
                    />
                    {resolvedProfile.origin_video_url ? (
                      <ToolbarButton
                        label="打开视频"
                        onClick={() => window.open(resolvedProfile.origin_video_url, "_blank", "noopener,noreferrer")}
                        disabled={pending}
                      />
                    ) : null}
                  </div>
                </div>
              ) : null}
            </div>
          </div>
        </article>

        <article className="rounded-[28px] border border-[var(--border)] bg-[var(--surface-soft)] p-5">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 className="m-0 text-xl font-semibold">订阅管理</h2>
              <p className="mt-2 text-sm leading-7 text-[var(--muted)]">
                支持单个更新、全部更新、删除订阅和查看订阅视频。
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <ToolbarButton
                label={busyKey === "refresh-subs" ? "刷新中..." : "刷新列表"}
                onClick={refreshSubscriptions}
                disabled={pending || busyKey === "refresh-subs"}
              />
              <ToolbarButton
                label={busyKey === "fetch-all" ? "更新中..." : "全部更新"}
                onClick={handleFetchAll}
                disabled={pending || subscriptions.length === 0 || busyKey === "fetch-all"}
                primary
              />
            </div>
          </div>

          {notice ? <NoticeBar notice={notice} /> : null}

          <div className="mt-5 grid gap-4 md:grid-cols-2">
            {subscriptions.map((item) => {
              const title = item.wx_nickname || item.wx_username || `订阅 #${item.id}`;
              const actionBusy =
                busyKey === `fetch:${item.id}` ||
                busyKey === `delete:${item.id}` ||
                busyKey.startsWith(`videos:${item.id}:`);

              return (
                <article key={item.id} className="rounded-[26px] border border-[var(--border)] bg-white p-5">
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex min-w-0 gap-3">
                      {item.wx_head_url ? (
                        <div className="relative h-14 w-14 overflow-hidden rounded-2xl border border-[var(--border)]">
                          <Image src={item.wx_head_url} alt={title} fill sizes="56px" className="object-cover" unoptimized />
                        </div>
                      ) : (
                        <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--surface-soft)] text-lg font-semibold text-[var(--primary)]">
                          {title.slice(0, 1)}
                        </div>
                      )}
                      <div className="min-w-0">
                        <div className="truncate text-lg font-medium">{title}</div>
                        <div className="mt-1 truncate text-xs text-[var(--muted)]">{item.wx_username || "未记录 finder"}</div>
                      </div>
                    </div>
                    <span className="rounded-full bg-[var(--surface-soft)] px-3 py-1 text-xs text-[var(--primary)]">
                      {item.video_count || 0} 条
                    </span>
                  </div>

                  <div className="mt-4 text-sm leading-7 text-[var(--muted)]">
                    {item.wx_signature || "暂无签名"}
                  </div>

                  <div className="mt-5 text-sm text-[var(--muted)]">
                    <div>最后更新：{formatDateTime(item.last_fetched_at)}</div>
                    <div className="mt-1">创建时间：{formatDateTime(item.created_at)}</div>
                  </div>

                  <div className="mt-5 flex flex-wrap gap-2">
                    <ToolbarButton
                      label={busyKey === `fetch:${item.id}` ? "更新中..." : "更新"}
                      onClick={() => handleFetchOne(item)}
                      disabled={pending || actionBusy}
                      primary
                    />
                    <ToolbarButton
                      label="查看视频"
                      onClick={() => loadVideos(item, 1)}
                      disabled={pending || actionBusy}
                    />
                    <ToolbarButton
                      label="删除"
                      onClick={() => handleDelete(item)}
                      disabled={pending || actionBusy}
                      danger
                    />
                  </div>
                </article>
              );
            })}
          </div>

          {subscriptions.length === 0 ? (
            <div className="mt-5 rounded-[24px] border border-dashed border-[var(--border)] px-5 py-10 text-sm text-[var(--muted)]">
              当前账号还没有订阅任何视频号。
            </div>
          ) : null}
        </article>
      </section>

      {videoPanel ? (
        <section className="rounded-[28px] border border-[var(--border)] bg-[var(--surface)] p-5 shadow-[0_20px_60px_rgba(20,38,18,0.08)]">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 className="m-0 text-xl font-semibold">视频列表 · {videoPanel.title}</h2>
              <p className="mt-2 text-sm leading-7 text-[var(--muted)]">
                第 {videoPanel.page} 页，共 {videoPanel.total} 条。这里直接显示 Hub 已存入数据库的视频记录。
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <ToolbarButton
                label="上一页"
                onClick={() => {
                  const current = subscriptions.find((item) => item.id === videoPanel.subscriptionId);
                  if (current && videoPanel.page > 1) {
                    loadVideos(current, videoPanel.page - 1);
                  }
                }}
                disabled={pending || videoPanel.page <= 1}
              />
              <ToolbarButton
                label="下一页"
                onClick={() => {
                  const current = subscriptions.find((item) => item.id === videoPanel.subscriptionId);
                  const maxPage = Math.max(Math.ceil(videoPanel.total / 20), 1);
                  if (current && videoPanel.page < maxPage) {
                    loadVideos(current, videoPanel.page + 1);
                  }
                }}
                disabled={pending || videoPanel.page >= Math.max(Math.ceil(videoPanel.total / 20), 1)}
              />
              <ToolbarButton label="收起" onClick={() => setVideoPanel(null)} />
            </div>
          </div>

          <div className="mt-5 space-y-4">
            {videoPanel.videos.map((video) => (
              <article key={video.id} className="rounded-[24px] border border-[var(--border)] bg-[var(--surface-soft)] p-4">
                <div className="flex flex-col gap-4 lg:flex-row">
                  {video.cover_url ? (
                    <div className="relative h-32 w-full overflow-hidden rounded-2xl border border-[var(--border)] lg:w-56">
                      <Image src={video.cover_url} alt={video.title || "视频封面"} fill sizes="224px" className="object-cover" unoptimized />
                    </div>
                  ) : null}
                  <div className="min-w-0 flex-1">
                    <div className="text-lg font-medium">{video.title || "未命名视频"}</div>
                    <div className="mt-2 text-sm leading-7 text-[var(--muted)]">{video.description || "暂无描述"}</div>
                    <div className="mt-4 grid gap-3 text-sm text-[var(--muted)] md:grid-cols-2 xl:grid-cols-3">
                      <div>发布时间：{formatDateTime(video.published_at)}</div>
                      <div>时长：{video.duration || 0} 秒</div>
                      <div>分辨率：{video.width || 0} × {video.height || 0}</div>
                      <div>点赞：{video.like_count || 0}</div>
                      <div>评论：{video.comment_count || 0}</div>
                      <div className="font-mono text-xs">OID：{video.object_id || "-"}</div>
                    </div>
                    <div className="mt-4 flex flex-wrap gap-2">
                      <ToolbarButton label="播放" onClick={() => handleOpenVideo(video)} primary />
                      <ToolbarButton label="复制链接" onClick={() => handleCopyVideo(video)} />
                      <ToolbarButton label="下载" onClick={() => handleDownloadVideo(video)} />
                    </div>
                  </div>
                </div>
              </article>
            ))}
          </div>

          {videoPanel.videos.length === 0 ? (
            <div className="mt-5 rounded-[24px] border border-dashed border-[var(--border)] px-5 py-10 text-sm text-[var(--muted)]">
              当前页没有视频数据。
            </div>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}

function buildProfileFromCompat(payload: HubSharedFeedCompatResponse): HubSharedFeedProfile {
  const compatPayload =
    payload?.data && "data" in payload.data && !("object" in payload.data)
      ? (payload.data as HubSharedFeedCompatResponse)
      : payload;

  const compatData = compatPayload?.data;
  const object = compatData?.object;
  const contact = object?.contact;
  const media = object?.objectDesc?.media?.[0];
  const feedInfo = compatData?.feedInfo;
  const authorInfo = compatData?.authorInfo;

  return {
    object_id: object?.id || "",
    wx_username: object?.username || contact?.username || "",
    wx_nickname: object?.nickname || contact?.nickname || authorInfo?.nickname || "",
    wx_head_url: object?.headUrl || contact?.headUrl || authorInfo?.headImgUrl || "",
    wx_signature: object?.signature || contact?.signature || "",
    description: object?.objectDesc?.description || feedInfo?.description || "",
    video_url: media?.url || feedInfo?.videoUrl || "",
    origin_video_url: feedInfo?.originVideoUrl || media?.url || "",
    cover_url: media?.coverUrl || media?.thumbUrl || feedInfo?.coverUrl || ""
  };
}

function hasResolvedProfile(profile: HubSharedFeedProfile) {
  return Boolean(
    profile.wx_username ||
      profile.wx_nickname ||
      profile.description ||
      profile.origin_video_url
  );
}

function buildPlayableVideoURL(video: HubVideo) {
  const rawURL = (video.video_url || "").trim();
  if (!rawURL) {
    return "";
  }

  const base = typeof window !== "undefined" ? window.location.origin : "";
  const url = new URL("/api/video/play", base || "http://localhost");
  url.searchParams.set("url", rawURL);

  const key = (video.decrypt_key || "").trim();
  if (key) {
    url.searchParams.set("key", key);
  }

  return base ? url.toString() : url.pathname + url.search;
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

function FieldInput({
  label,
  value,
  onChange,
  placeholder
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}) {
  return (
    <label className="block">
      <span className="mb-2 block text-sm text-[var(--muted)]">{label}</span>
      <input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className="w-full rounded-2xl border border-[var(--border)] bg-white px-4 py-3 text-sm text-[var(--text)] outline-none transition focus:border-[var(--primary)]"
      />
    </label>
  );
}

function FieldTextarea({
  label,
  value,
  onChange,
  placeholder
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}) {
  return (
    <label className="block">
      <span className="mb-2 block text-sm text-[var(--muted)]">{label}</span>
      <textarea
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        rows={4}
        className="w-full resize-y rounded-2xl border border-[var(--border)] bg-white px-4 py-3 text-sm text-[var(--text)] outline-none transition focus:border-[var(--primary)]"
      />
    </label>
  );
}

function ToolbarButton({
  label,
  onClick,
  disabled,
  primary = false,
  danger = false
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  primary?: boolean;
  danger?: boolean;
}) {
  const className = danger
    ? "border-[var(--danger)]/25 text-[var(--danger)] hover:bg-[var(--danger)]/8"
    : primary
      ? "border-[var(--primary)] bg-[var(--primary)] text-white hover:bg-[var(--primary-strong)]"
      : "border-[var(--border)] bg-white text-[var(--text)] hover:border-[var(--primary)] hover:text-[var(--primary)]";

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={`rounded-full border px-4 py-2 text-sm transition disabled:cursor-not-allowed disabled:opacity-60 ${className}`}
    >
      {label}
    </button>
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
