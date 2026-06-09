export const HUB_TOKEN_COOKIE = "hub_token";

export class HubConfigError extends Error {
  constructor(message = "HUB_API_BASE is not configured") {
    super(message);
    this.name = "HubConfigError";
  }
}

export class HubUnauthorizedError extends Error {
  constructor(message = "Hub authentication required") {
    super(message);
    this.name = "HubUnauthorizedError";
  }
}

export type HubUser = {
  id?: number;
  email?: string;
  username?: string;
  role?: string;
};

export type HubClient = {
  id: string;
  hostname?: string;
  display_name?: string;
  status?: string;
  version?: string;
  ip?: string;
  port?: number;
  page_path?: string;
  href?: string;
  api_ready?: boolean;
  supports_search?: boolean;
  supports_feed?: boolean;
  supports_profile?: boolean;
  user_id?: number;
  bind_status?: boolean;
  is_locked?: boolean;
  device_group?: string;
  last_seen?: string;
  created_at?: string;
  updated_at?: string;
};

export type HubTask = {
  id: number;
  type?: string;
  node_id?: string;
  status?: string;
  created_at?: string;
};

export type HubSubscription = {
  id: number;
  wx_username?: string;
  wx_nickname?: string;
  wx_signature?: string;
  wx_head_url?: string;
  status?: string;
  video_count?: number;
  last_fetched_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type HubSharedFeedProfile = {
  wx_username?: string;
  wx_nickname?: string;
  wx_head_url?: string;
  wx_signature?: string;
  description?: string;
  object_id?: string;
  video_url?: string;
  origin_video_url?: string;
  cover_url?: string;
};

export type HubDeviceListResponse = {
  code?: number;
  devices?: HubClient[];
  message?: string;
};

export type HubVideo = {
  id: number;
  subscription_id: number;
  object_id?: string;
  object_nonce_id?: string;
  title?: string;
  cover_url?: string;
  description?: string;
  duration?: number;
  width?: number;
  height?: number;
  like_count?: number;
  comment_count?: number;
  video_url?: string;
  decrypt_key?: string;
  published_at?: string;
  created_at?: string;
};

export type HubSubscriptionVideosResponse = {
  code?: number;
  data?: {
    videos?: HubVideo[];
    total?: number;
    page?: number;
  };
  message?: string;
};

export type HubActionResult = {
  code?: number;
  success?: boolean;
  message?: string;
  token?: string;
  data?: Record<string, unknown>;
};

export type HubSharedFeedCompatResponse = {
  code?: number;
  message?: string;
  errCode?: number;
  errMsg?: string;
  data?: {
    errCode?: number;
    errMsg?: string;
    data?: {
      object?: {
        id?: string;
        username?: string;
        nickname?: string;
        headUrl?: string;
        signature?: string;
        objectDesc?: {
          description?: string;
          media?: Array<{
            url?: string;
            coverUrl?: string;
            thumbUrl?: string;
            decodeKey?: string;
          }>;
        };
        contact?: {
          username?: string;
          nickname?: string;
          headUrl?: string;
          signature?: string;
        };
      };
      feedInfo?: {
        videoUrl?: string;
        originVideoUrl?: string;
        description?: string;
        coverUrl?: string;
      };
      authorInfo?: {
        nickname?: string;
        headImgUrl?: string;
      };
    };
    object?: {
      id?: string;
      username?: string;
      nickname?: string;
      headUrl?: string;
      signature?: string;
      objectDesc?: {
        description?: string;
        media?: Array<{
          url?: string;
          coverUrl?: string;
          thumbUrl?: string;
          decodeKey?: string;
        }>;
      };
      contact?: {
        username?: string;
        nickname?: string;
        headUrl?: string;
        signature?: string;
      };
    };
    feedInfo?: {
      videoUrl?: string;
      originVideoUrl?: string;
      description?: string;
      coverUrl?: string;
    };
    authorInfo?: {
      nickname?: string;
      headImgUrl?: string;
    };
  };
};

export type HubDashboardData = {
  clients: HubClient[];
  tasks: HubTask[];
  subscriptions: HubSubscription[];
};

function trimSlash(value: string): string {
  return value.replace(/\/+$/, "");
}

export function getHubApiBase(): string {
  const envValue =
    process.env.HUB_API_BASE?.trim() ||
    process.env.NEXT_PUBLIC_HUB_API_BASE?.trim();
  if (envValue) {
    return trimSlash(envValue);
  }
  throw new HubConfigError();
}

export function getBearerTokenFallback(): string | null {
  return process.env.HUB_DEMO_TOKEN?.trim() || process.env.NEXT_PUBLIC_HUB_DEMO_TOKEN?.trim() || null;
}

export async function hubFetch<T>(path: string, init?: RequestInit, token?: string | null): Promise<T> {
  const headers = new Headers(init?.headers || {});
  headers.set("Content-Type", "application/json");

  const bearerToken = token || getBearerTokenFallback();
  if (!bearerToken) {
    throw new HubUnauthorizedError();
  }
  headers.set("Authorization", `Bearer ${bearerToken}`);

  const response = await fetch(`${getHubApiBase()}${path}`, {
    ...init,
    headers,
    cache: "no-store"
  });

  if (response.status === 401) {
    throw new HubUnauthorizedError();
  }

  if (!response.ok) {
    const text = await response.text();
    throw new Error(`Hub API ${response.status}: ${text || response.statusText}`);
  }

  return response.json() as Promise<T>;
}

export async function getProfile(token?: string | null): Promise<HubUser> {
  return hubFetch<HubUser>("/api/auth/profile", undefined, token);
}

export async function getDashboardData(token?: string | null): Promise<HubDashboardData> {
  const [clients, tasks, subscriptions] = await Promise.all([
    hubFetch<HubClient[]>("/api/clients", undefined, token),
    hubFetch<{ list?: HubTask[] }>("/api/tasks?offset=0&limit=8", undefined, token),
    hubFetch<{ code?: number; data?: HubSubscription[] }>("/api/subscriptions", undefined, token)
  ]);

  return {
    clients,
    tasks: tasks.list || [],
    subscriptions: subscriptions.data || []
  };
}

export async function getClients(token?: string | null): Promise<HubClient[]> {
  return hubFetch<HubClient[]>("/api/clients", undefined, token);
}

export async function getUserDevices(token?: string | null): Promise<HubClient[]> {
  const result = await hubFetch<HubDeviceListResponse>("/api/device/list", undefined, token);
  return result.devices || [];
}

export async function getTasks(token?: string | null): Promise<HubTask[]> {
  const result = await hubFetch<{ list?: HubTask[] }>("/api/tasks?offset=0&limit=30", undefined, token);
  return result.list || [];
}

export async function getSubscriptions(token?: string | null): Promise<HubSubscription[]> {
  const result = await hubFetch<{ code?: number; data?: HubSubscription[] }>("/api/subscriptions", undefined, token);
  return result.data || [];
}

export async function getSubscriptionVideos(
  id: number,
  page = 1,
  token?: string | null
): Promise<HubSubscriptionVideosResponse["data"]> {
  const result = await hubFetch<HubSubscriptionVideosResponse>(
    `/api/subscriptions/${id}/videos?page=${page}`,
    undefined,
    token
  );
  return result.data;
}
