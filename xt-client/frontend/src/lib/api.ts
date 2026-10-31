export interface QuotaItem {
  label: string;
  used?: number | null;
  total?: number | null;
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
  startLogin: () => request<LoginSession>('/api/auth/trae', { method: 'POST' }),
  pollLogin: (id: string) => request<{ status: string; accountId?: string; nickname?: string; error?: string }>(`/api/auth/trae/${id}`),
  cancelLogin: (id: string) => request<{ ok: boolean }>(`/api/auth/trae/${id}`, { method: 'DELETE' }),
  updateAccount: (id: string, body: { alias?: string; enabled?: boolean }) =>
    request<Account>(`/api/accounts/${id}/rename`, { method: 'POST', body: JSON.stringify(body) }),
  deleteAccount: (id: string) => request<{ ok: boolean }>(`/api/accounts/${id}/delete`, { method: 'POST' }),
  refreshQuota: (id: string) => request<QuotaSnapshot>(`/api/accounts/${id}/quota`, { method: 'POST' }),
  proxyKey: () => request<{ key: string }>('/api/proxy/key'),
  rotateProxyKey: () => request<{ key: string }>('/api/proxy/key', { method: 'POST' }),
  saveProxy: (body: { defaultAccountId?: string }) =>
    request<ProxySettings>('/api/proxy/settings', { method: 'PUT', body: JSON.stringify(body) }),
};
