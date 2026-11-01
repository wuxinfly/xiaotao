export interface QuotaItem {
  label: string;
  used?: number | null;
  total?: number | null;
  remaining?: number | null;
  unit: string;
  expireAt?: number;
}

export interface QuotaSnapshot {
  status: string;
  items: QuotaItem[];
  error?: string;
  fetchedAt: number;
}

export interface Account {
  id: string;
  provider: string;
  region: string;
  uid: string;
  nickname: string;
  alias: string;
  enabled: boolean;
  authState: string;
  addedAt: number;
  quota?: QuotaSnapshot | null;
  usageTokens: number;
  usageCalls: number;
}

export interface ProxySettings {
  defaultAccountId: string;
}

export interface ProviderInfo {
  id: string;
  displayName: string;
  loginType: string;
  regions: string[];
  billing: string;
}

export interface LoginSession {
  id: string;
  status: string;
  loginUrl: string;
  error?: string;
}

export interface LaunchInfo {
  startedAt: number;
  uptimeSeconds: number;
  batchId: string;
}

export interface SessionStats {
  totalTokens: number;
  promptTokens: number;
  completionTokens: number;
  cachedTokens: number;
}

export interface DailyUsage {
  date: string;
  promptTokens: number;
  completionTokens: number;
  cachedTokens: number;
  totalRequests: number;
  completedRequests: number;
  failedRequests: number;
  interruptedRequests: number;
  missingUsageRequests: number;
}

export interface CumulativeStats {
  totalTokens: number;
  promptTokens: number;
  completionTokens: number;
  cachedTokens: number;
  totalRequests: number;
}

export interface TokenMetrics {
  prompt?: number | null;
  completion?: number | null;
  cached?: number | null;
  total?: number | null;
}

export interface RequestSummary {
  id: string;
  channel: string;
  channelLabel: string;
  model: string;
  actionTag: string;
  actionTitle: string;
  status: string;
  statusText: string;
  startedAt: number;
  firstTokenAt?: number | null;
  endedAt?: number | null;
  durationMs: number;
  firstTokenDurationMs?: number | null;
  tokens: TokenMetrics;
  speedTokPerSec?: number | null;
  errorSummary?: string;
}

export interface HeatmapDay {
  totalTokens: number;
  promptTokens: number;
  completionTokens: number;
  cachedTokens: number;
  totalRequests: number;
  hasMissingUsage: boolean;
}

export interface HeatmapData {
  startDate: string;
  endDate: string;
  trackingStartDate: string;
  days: Record<string, HeatmapDay>;
}

export interface StorageStatus {
  status: 'healthy' | 'degraded' | 'error';
  message?: string;
}

export interface DashboardSnapshot {
  version: string;
  launch: LaunchInfo;
  currentSession: SessionStats;
  today: DailyUsage;
  cumulative: CumulativeStats;
  recentRequests: RequestSummary[];
  activeCount: number;
  activeRequests: RequestSummary[];
  heatmap: HeatmapData;
  storage: StorageStatus;
  versionSeq: number;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });
  if (!res.ok) {
    let message = `请求失败（${res.status}）`;
    try {
      const body = await res.json();
      if (body?.error?.message) message = body.error.message;
    } catch { /* keep default */ }
    throw new Error(message);
  }
  return res.json() as Promise<T>;
}

export const api = {
  providers: () => request<{ providers: ProviderInfo[] }>('/api/providers'),
  accounts: () => request<{ accounts: Account[]; proxy: ProxySettings }>('/api/accounts'),
  startLogin: (provider = 'trae') => request<LoginSession>(`/api/auth/${provider}`, { method: 'POST' }),
  pollLogin: (id: string, provider = 'trae') =>
    request<{ status: string; accountId?: string; nickname?: string; error?: string }>(`/api/auth/${provider}/${id}`),
  cancelLogin: (id: string, provider = 'trae') => request<{ ok: boolean }>(`/api/auth/${provider}/${id}`, { method: 'DELETE' }),
  updateAccount: (id: string, body: { alias?: string; enabled?: boolean }) =>
    request<Account>(`/api/accounts/${id}/rename`, { method: 'POST', body: JSON.stringify(body) }),
  deleteAccount: (id: string) => request<{ ok: boolean }>(`/api/accounts/${id}/delete`, { method: 'POST' }),
  refreshQuota: (id: string) => request<QuotaSnapshot>(`/api/accounts/${id}/quota`, { method: 'POST' }),
  proxyKey: () => request<{ key: string }>('/api/proxy/key'),
  rotateProxyKey: () => request<{ key: string }>('/api/proxy/key', { method: 'POST' }),
  saveProxy: (body: { defaultAccountId?: string }) =>
    request<ProxySettings>('/api/proxy/settings', { method: 'PUT', body: JSON.stringify(body) }),
  dashboard: () => request<DashboardSnapshot>('/api/dashboard'),
  subscribeDashboard: (
    onSnapshot: (snap: DashboardSnapshot) => void,
    onError?: (err: Event) => void,
  ): (() => void) => {
    const es = new EventSource('/api/dashboard/events');
    const handleMsg = (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data) as DashboardSnapshot;
        onSnapshot(data);
      } catch { /* ignore */ }
    };
    es.addEventListener('snapshot', handleMsg);
    es.addEventListener('update', handleMsg);
    if (onError) {
      es.onerror = onError;
    }
    return () => es.close();
  },
};
