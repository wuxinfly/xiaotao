import type {AgentModelSettings} from './agent-model-settings';
import type {ContextBreakdown} from '../features/agent/contextTypes';
export type {AgentDefaultModels, AgentModelConfig, AgentModelDefaults, AgentModelSettings} from './agent-model-settings';
export type {ContextBreakdown};

export interface ReviewScope {
  cutoffPositions: Record<string, number>;
  runIds: string[];
  availableCount: number;
  includedCount: number;
  truncation?: string[];
}

export interface ReviewEvidenceItem {
  id: string;
  kind: string;
  runId: string;
  sourceId: string;
  summary: string;
  truncated?: boolean;
}

export interface ReviewIssue {
  id: string;
  title: string;
  problem: string;
  impact: string;
  speculation: string;
  proposal: string;
  acceptance: string;
  evidenceIds: string[];
}

export interface ReviewGoodPractice {
  id: string;
  title: string;
  description: string;
  evidenceIds?: string[];
}

export interface ReviewResult {
  outcome: 'issues_found' | 'no_issues_found' | 'insufficient_evidence' | string;
  summary: string;
  issues: ReviewIssue[];
  goodPractices: ReviewGoodPractice[];
  limitations: string[];
}

export interface ReviewUsage {
  promptTokens?: number;
  completionTokens: number;
  totalTokens: number;
}

export interface ReviewSuggestionLink {
  issueId: string;
  issueKey: string;
  observationId: string;
  linkedAt: string;
  status: string;
  sourcePath: string;
}

export interface BehaviorReview {
  id: string;
  schemaVersion: number;
  sessionId: string;
  projectSnapshot: string;
  sessionTitleSnapshot: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
  timezone: string;
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled';
  requestId: string;
  attemptOf?: string;
  model: string;
  clientVersion: string;
  coreVersion: string;
  promptVersion: string;
  scope: ReviewScope;
  evidenceSnapshot: ReviewEvidenceItem[];
  result?: ReviewResult;
  errorCode?: string;
  errorMessage?: string;
  usage?: ReviewUsage;
  suggestionLinks?: ReviewSuggestionLink[];
}

export interface ReviewSummary {
  id: string;
  sessionId: string;
  sessionTitleSnapshot: string;
  projectSnapshot: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
  timezone: string;
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled';
  title: string;
  model: string;
  attemptOf?: string;
  errorCode?: string;
  errorMessage?: string;
  outcome?: string;
  issuesCount: number;
}

export interface ReviewListPage {
  reviews: ReviewSummary[];
  nextCursor?: string;
  total: number;
}

export interface ExistingSuggestionItem {
  issueKey: string;
  title: string;
  problem: string;
  status: string;
  observationsCount: number;
  updatedAt: string;
}

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

export interface CheckinStatus {
  supported: boolean;
  active: boolean;
  checkedIn: boolean;
  streakDays?: number;
  dailyCredit?: number;
  message?: string;
  fetchedAt?: number;
}

export interface CheckinClaimResult {
  error?: string;
  ok: boolean;
  checkedIn: boolean;
  extraCredits?: number;
  streakDays?: number;
  message?: string;
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
  checkin?: CheckinStatus | null;
  usageTokens: number;
  usageCalls: number;
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
  accountId: string;
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

export interface AgentProject {
  isDefault?: boolean;
  saved?: boolean;
  // Set when the project entry was deleted: the session keeps its working
  // directory but is grouped under the recent list.
  removed?: boolean;
  path: string;
  name: string;
}

export interface ModelPricing {
 priceFactor?: number;
 isFree?: boolean;
}

export interface AgentModelOption extends ModelPricing {
  model: string;
  label: string;
  supportsImages?: boolean;
  contextWindow?: number;
}

export interface AgentImage {
  id: string;
  mimeType: string;
  size: number;
  url: string;
  width?: number;
  height?: number;
  createdAt: string;
}

export interface AgentMessage {
  toolCalls?: Array<Record<string, unknown>>;
  id: string;
  role: 'user' | 'assistant' | 'tool';
  content: string;
  images?: AgentImage[];
  toolCallId?: string;
  toolName?: string;
  references?: AgentConversationReference[];
  createdAt: string;
  runId?: string;
}

export interface AgentConversationReference {
  id: string;
  sourceSessionId: string;
  title: string;
  projectName?: string;
  summary?: string;
  lastPosition: number;
  truncated?: boolean;
  sourceAvailable?: boolean;
  hasNewContent?: boolean;
  sourceUpdatedAt: string;
  createdAt: string;
}

export interface AgentReferenceCandidate {
  sessionId: string;
  title: string;
  project: AgentProject;
  updatedAt: string;
}

export interface AgentConversationReferenceDetail {
  reference: AgentConversationReference;
  summary: string;
  sourceAvailable: boolean;
}

export interface AgentActivity {
  id: string;
  runId?: string;
  kind: string;
  title: string;
  detail?: string;
  tool?: string;
  input?: string;
  output?: string;
  status: 'running' | 'completed' | 'failed';
  startedAt: string;
  endedAt?: string;
}

export interface AgentThought {
  id: string;
  runId: string;
  kind: 'understanding' | 'observation' | 'decision' | 'checkpoint';
  text: string;
  status: 'running' | 'completed';
  createdAt: string;
}

export interface AgentRunStats {
  phase?: 'working' | 'completed' | 'failed' | 'canceled';
  runId: string;
  userMessageId: string;
  startedAt: string;
  endedAt?: string;
  filesRead: number;
  filesWritten: number;
  toolCalls: number;
  modelRequests?: number;
  outputTokens?: number;
  inputTokens?: number;
  inputKnown?: boolean;
  cachedTokens?: number;
  cacheKnown?: boolean;
  breakdown?: ContextBreakdown;
  outputSpeed?: number;
  firstTokenMs?: number;
  liveTokens?: number;
  liveSpeed?: number;
  contextTokens: number;
  contextWindow: number;
}

export interface AgentSession {
  id: string;
  project: AgentProject;
  title: string;
  model: string;
  modelSettings?: AgentModelSettings;
  status: 'idle' | 'running' | 'completed' | 'failed' | 'canceled';
  messages: AgentMessage[];
  activity: AgentActivity[];
  thoughts: AgentThought[];
  runStats: AgentRunStats[];
  streamDrafts?: AgentMessage[];
  hasImages?: boolean;
  error?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AgentEvent {
  runId?: string;
  messageId?: string;
  type: 'answer_end' | 'session' | 'message' | 'message_delta' | 'activity' | 'thought' | 'run_stats' | 'status' | 'error' | 'approval';
  approval?: {id: string; tool: string; detail: string};
  sessionId: string;
  message?: AgentMessage;
  activity?: AgentActivity;
  thought?: AgentThought;
  runStats?: AgentRunStats;
  status?: AgentSession['status'];
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

async function dashboardRequest(): Promise<DashboardSnapshot> {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), 10_000);
  try {
    return await request<DashboardSnapshot>('/api/dashboard', { signal: controller.signal });
  } catch (error) {
    if (controller.signal.aborted) {
      throw new Error('仪表盘响应超时，请重试', {cause: error});
    }
    throw error;
  } finally {
    window.clearTimeout(timer);
  }
}

export interface ModelCatalog {
  models: ({ id: string; displayName?: string; multimodal: boolean } & ModelPricing)[];
  source: 'live' | 'builtin' | 'merged' | 'cached' | 'stale' | 'error';
  error?: string;
}

export interface MCPServer { command: string; args: string[]; disabled?: boolean }
export interface MCPSettings { mcpServers: Record<string, MCPServer>; source?: 'local' | 'environment' | 'empty' }
export interface SkillDirectory {path: string; disabled?: boolean}
export interface SkillsSettings {directories: SkillDirectory[]; disabledSkills: string[]}
export interface LocalSkill {id: string; name: string; description: string; path: string; directory: string; enabled: boolean; error?: string}
export interface SkillsCatalog {settings: SkillsSettings; skills: LocalSkill[]; issues: {path: string; message: string}[]; issuesOmitted?: number}

export const api = {
  agentSkills: (signal?: AbortSignal) => request<SkillsCatalog>('/api/agent/skills', {signal}),
  scanAgentSkills: (settings: SkillsSettings, signal?: AbortSignal) => request<SkillsCatalog>('/api/agent/skills/scan', {method:'POST', body:JSON.stringify(settings), signal}),
  saveAgentSkills: (settings: SkillsSettings, signal?: AbortSignal) => request<SkillsCatalog>('/api/agent/skills', {method:'PUT', body:JSON.stringify(settings), signal}),
  agentMCP: (signal?: AbortSignal) => request<MCPSettings>('/api/agent/mcp', {signal}),
  saveAgentMCP: (settings: MCPSettings) => request<{ok: boolean}>('/api/agent/mcp', {method:'PUT', body:JSON.stringify(settings)}),
  testAgentMCP: (server: MCPServer, signal?: AbortSignal) => request<{tools: string[]}>('/api/agent/mcp/test', {method:'POST', body:JSON.stringify(server), signal}),
  accountModels: (id: string, refresh = false) => request<ModelCatalog>(`/api/proxy/models?accountId=${encodeURIComponent(id)}`, { method: refresh ? 'POST' : 'GET' }),
  providers: () => request<{ providers: ProviderInfo[] }>('/api/providers'),
  accounts: () => request<{ accounts: Account[] }>('/api/accounts'),
  startLogin: (provider = 'trae', region = 'cn') => request<LoginSession>(`/api/auth/${provider}`, { method: 'POST', body: JSON.stringify({ region }) }),
  pollLogin: (id: string, provider = 'trae') =>
    request<{ status: string; accountId?: string; nickname?: string; error?: string }>(`/api/auth/${provider}/${id}`),
  cancelLogin: (id: string, provider = 'trae') => request<{ ok: boolean }>(`/api/auth/${provider}/${id}`, { method: 'DELETE' }),
  importCodexAuth: (payload: string) =>
    request<{ status: string; accountId: string; nickname: string }>('/api/auth/codex/import', { method: 'POST', body: payload }),
  submitCodexCallback: (sessionId: string, callback: string) =>
    request<{ status: string; accountId: string; nickname: string }>(`/api/auth/codex/${sessionId}/callback`, { method: 'POST', body: JSON.stringify({ callback }) }),
  updateAccount: (id: string, body: { alias?: string; enabled?: boolean }) =>
    request<Account>(`/api/accounts/${id}/rename`, { method: 'POST', body: JSON.stringify(body) }),
  reorderAccounts: (providerId: string, accountIds: string[]) =>
    request<{ accounts: Account[] }>('/api/accounts/order', {
      method: 'POST',
      body: JSON.stringify({ providerId, accountIds }),
    }),
  deleteAccount: (id: string) => request<{ ok: boolean }>(`/api/accounts/${id}/delete`, { method: 'POST' }),
  refreshQuota: (id: string) => request<QuotaSnapshot>(`/api/accounts/${id}/quota`, { method: 'POST' }),
  getCheckinStatus: (id: string) => request<CheckinStatus>(`/api/accounts/${id}/checkin`),
  claimCheckin: (id: string) => request<CheckinClaimResult>(`/api/accounts/${id}/checkin`, { method: 'POST' }),
  claimAllCheckin: (provider?: string) => request<{ results: Record<string, CheckinClaimResult> }>(`/api/checkin/claim-all${provider ? '?provider=' + encodeURIComponent(provider) : ''}`, { method: 'POST' }),
  proxyKey: () => request<{ key: string }>('/api/proxy/key'),
  rotateProxyKey: () => request<{ key: string }>('/api/proxy/key', { method: 'POST' }),
  dashboard: dashboardRequest,
  requestHudEnabled: () => request<{ enabled: boolean }>('/api/request-hud'),
  setRequestHudEnabled: (enabled: boolean) =>
    request<{ enabled: boolean; error?: string }>('/api/request-hud', {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
  agentSessions: () => request<{ sessions: AgentSession[] }>('/api/agent/sessions?summary=1'),
  agentProjects: () => request<{ projects: AgentProject[] }>('/api/agent/projects'),
  saveAgentProject: (path: string, name: string) => request('/api/agent/projects', {method: 'POST', body: JSON.stringify({path, name})}),
  deleteAgentProject: (path: string) => request('/api/agent/projects', {method: 'DELETE', body: JSON.stringify({path})}),
  renameAgentProject: (path: string, name: string) => request('/api/agent/project/name', {method: 'PATCH', body: JSON.stringify({path, name})}),
  openAgentProject: (path: string) => request('/api/agent/project/open', {method: 'POST', body: JSON.stringify({path})}),
  renameAgentSession: (id: string, title: string) => request<AgentSession>('/api/agent/sessions/' + encodeURIComponent(id), {method: 'PATCH', body: JSON.stringify({title})}),
  generateAgentSessionTitle: (id: string, signal?: AbortSignal, model?: string, modelSettings?: AgentModelSettings) => request<{title: string}>('/api/agent/sessions/' + encodeURIComponent(id) + '/title', {method: 'POST', body: JSON.stringify({model, modelSettings}), signal}),
  deleteAgentSession: (id: string) => request<{ok: boolean}>('/api/agent/sessions/' + encodeURIComponent(id), {method: 'DELETE'}),
  agentModels: (signal?: AbortSignal, refresh = false) => request<{ models: AgentModelOption[]; errors?: string[] }>(refresh ? '/api/agent/models/refresh' : '/api/agent/models', {signal, method: refresh ? 'POST' : 'GET'}),
  agentHistory: (id: string, before = '') => request<{session: AgentSession; before: string}>(`/api/agent/sessions/${encodeURIComponent(id)}/history?before=${encodeURIComponent(before)}`),
  agentReferenceSessions: (query: string, scope: 'project' | 'all', projectPath: string, exclude: string[] = []) => {
    const params = new URLSearchParams({query, scope, projectPath});
    for (const id of exclude) params.append('exclude', id);
    return request<{sessions: AgentReferenceCandidate[]}>(`/api/agent/references/sessions?${params}`);
  },
  agentReference: (id: string) => request<AgentConversationReferenceDetail>(`/api/agent/references/${encodeURIComponent(id)}`),
  agentSession: (id: string) => request<AgentSession>(`/api/agent/sessions/${encodeURIComponent(id)}`),
  createAgentSession: (projectPath: string, model: string, projectName = '', modelSettings?: AgentModelSettings) =>
    request<AgentSession>('/api/agent/sessions', {
      method: 'POST',
      body: JSON.stringify({ projectPath, projectName, model, modelSettings }),
    }),
  resolveAgentProject: (path: string) =>
    request<AgentProject>('/api/agent/project', {
      method: 'POST',
      body: JSON.stringify({ path }),
    }),
  cancelAgentSession: (id: string) =>
    request<{ ok: boolean }>(`/api/agent/sessions/${encodeURIComponent(id)}/cancel`, { method: 'POST' }),
  createReview: (sessionId: string, requestId?: string, model?: string, modelSettings?: AgentModelSettings) =>
    request<BehaviorReview>(`/api/agent/sessions/${encodeURIComponent(sessionId)}/reviews`, {
      method: 'POST',
      body: JSON.stringify({ requestId, model, modelSettings }),
    }),
  listReviews: (sessionId?: string, cursor?: string, limit?: number) => {
    const params = new URLSearchParams();
    if (sessionId) params.set('sessionId', sessionId);
    if (cursor) params.set('cursor', cursor);
    if (limit) params.set('limit', String(limit));
    const qs = params.toString();
    return request<ReviewListPage>(`/api/agent/reviews${qs ? '?' + qs : ''}`);
  },
  getReview: (id: string) => request<BehaviorReview>(`/api/agent/reviews/${encodeURIComponent(id)}`),
  cancelReview: (id: string) =>
    request<{ ok: boolean }>(`/api/agent/reviews/${encodeURIComponent(id)}/cancel`, { method: 'POST' }),
  retryReview: (id: string, requestId?: string) =>
    request<BehaviorReview>(`/api/agent/reviews/${encodeURIComponent(id)}/retry`, {
      method: 'POST',
      body: JSON.stringify({ requestId }),
    }),
  deleteReview: (id: string) =>
    request<{ ok: boolean }>(`/api/agent/reviews/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  addIssueSuggestion: (id: string, issueId: string, issueKey?: string) =>
    request<ReviewSuggestionLink>(`/api/agent/reviews/${encodeURIComponent(id)}/issues/${encodeURIComponent(issueId)}/suggestion`, {
      method: 'POST',
      body: JSON.stringify({ issueKey }),
    }),
  getReviewProjectSuggestions: (id: string) =>
    request<{ suggestions: ExistingSuggestionItem[] }>(`/api/agent/reviews/${encodeURIComponent(id)}/suggestions`),
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
