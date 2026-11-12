import {request} from './http';
import type {AgentActivity, AgentMessage, AgentRunStats, AgentSession, AgentThought} from './api';

export type OfficeTaskStatus = 'queued' | 'running' | 'waiting' | 'completed' | 'failed' | 'canceled';
export type OfficeMessageStatus = 'queued' | 'delivered' | 'failed';

export interface OfficeExecutor {
  kind: string;
  name: string;
  role: string;
  model: string;
  modelLabel?: string;
  sessionId?: string;
}

export interface OfficeApproval {
  id: string;
  tool: string;
  detail: string;
}

export interface OfficeActivity {
  kind: string;
  title: string;
  detail?: string;
  status: 'running' | 'completed' | 'failed';
  startedAt: string;
  endedAt?: string;
}

export interface OfficeStats {
  toolCalls: number;
  filesRead: number;
  filesWritten: number;
  modelRequests?: number;
  outputTokens?: number;
  inputTokens?: number;
  inputKnown?: boolean;
  cachedTokens?: number;
  cacheKnown?: boolean;
  contextTokens?: number;
  contextWindow?: number;
  firstTokenMs?: number;
  liveTokens?: number;
  durationMs?: number;
}

export interface OfficeTask {
  id: string;
  projectPath: string;
  projectName: string;
  title: string;
  goal: string;
  status: OfficeTaskStatus;
  error?: string;
  cancelRequested?: boolean;
  waitingReason?: string;
  approval?: OfficeApproval;
  executor: OfficeExecutor;
  sessionId?: string;
  runId?: string;
  stats: OfficeStats;
  latestActivity?: OfficeActivity;
  queuedMessages?: number;
  createdAt: string;
  updatedAt: string;
  startedAt?: string;
  endedAt?: string;
}

export interface OfficeArtifact {
  id: string;
  taskId: string;
  taskTitle: string;
  projectPath: string;
  path: string;
  size: number;
  sha256?: string;
  tool: string;
  runId?: string;
  activityId?: string;
  createdAt: string;
  updatedAt: string;
}

export interface OfficeArtifactDetail {
  artifact: OfficeArtifact;
  available: boolean;
  binary?: boolean;
  truncated?: boolean;
  content?: string;
  error?: string;
}

export interface OfficeMember {
  id: string;
  name: string;
  role: 'coordinator' | 'executor' | string;
  status: string;
  model?: string;
  taskId?: string;
  taskTitle?: string;
  taskStatus?: OfficeTaskStatus;
  todayTasks: number;
  latestActivity?: OfficeActivity;
}

export interface OfficeCapabilities {
  midRunMessages: boolean;
  queuedMessages: boolean;
  approval: boolean;
  restartResume: boolean;
  sameProjectIsolation: boolean;
  commandArtifacts: boolean;
  artifactTools: string[];
  notes: string[];
}

export interface OfficeSnapshot {
  project: {path: string; name: string};
  accessMode: 'safe' | 'full';
  tasks: OfficeTask[];
  members: OfficeMember[];
  artifacts: OfficeArtifact[];
  capabilities: OfficeCapabilities;
}

export interface OfficeTaskMessage {
  id: string;
  taskId: string;
  content: string;
  status: OfficeMessageStatus;
  error?: string;
  createdAt: string;
  deliveredAt?: string;
}

export interface OfficeHistory {
  session: AgentSession;
  before: string;
}

export interface OfficeTaskDetail {
  task: OfficeTask;
  history?: OfficeHistory;
  historyError?: string;
  artifacts: OfficeArtifact[];
  messages: OfficeTaskMessage[];
}

export interface CreateOfficeTaskInput {
  projectPath: string;
  projectName: string;
  title: string;
  goal: string;
  model: string;
  modelSettings?: {contextWindow?: number; maxOutputTokens?: number; reasoningEffort?: string};
}

function projectQuery(projectPath: string) {
  return `projectPath=${encodeURIComponent(projectPath)}`;
}

export const officeApi = {
  snapshot: (projectPath: string, projectName = '', signal?: AbortSignal) =>
    request<OfficeSnapshot>(`/api/office/snapshot?${projectQuery(projectPath)}${projectName ? '&projectName=' + encodeURIComponent(projectName) : ''}`, {signal}),
  createTask: (input: CreateOfficeTaskInput) =>
    request<OfficeTask>('/api/office/tasks', {method: 'POST', body: JSON.stringify(input)}),
  taskDetail: (projectPath: string, id: string, signal?: AbortSignal) =>
    request<OfficeTaskDetail>(`/api/office/tasks/${encodeURIComponent(id)}?${projectQuery(projectPath)}`, {signal}),
  taskHistory: (projectPath: string, id: string, before = '') =>
    request<OfficeHistory>(`/api/office/tasks/${encodeURIComponent(id)}/history?${projectQuery(projectPath)}&before=${encodeURIComponent(before)}`),
  cancelTask: (projectPath: string, id: string) =>
    request<{ok: boolean}>(`/api/office/tasks/${encodeURIComponent(id)}/cancel?${projectQuery(projectPath)}`, {method: 'POST'}),
  postMessage: (projectPath: string, id: string, content: string) =>
    request<OfficeTaskMessage>(`/api/office/tasks/${encodeURIComponent(id)}/messages?${projectQuery(projectPath)}`, {method: 'POST', body: JSON.stringify({content})}),
  resolveApproval: (projectPath: string, id: string, approvalId: string, approved: boolean) =>
    request<{ok: boolean}>(`/api/office/tasks/${encodeURIComponent(id)}/approval?${projectQuery(projectPath)}`, {method: 'POST', body: JSON.stringify({id: approvalId, approved})}),
  artifact: (projectPath: string, id: string) =>
    request<OfficeArtifactDetail>(`/api/office/artifacts/${encodeURIComponent(id)}?${projectQuery(projectPath)}`),
};

export type {AgentActivity, AgentMessage, AgentRunStats, AgentSession, AgentThought};

export const taskStatusLabel: Record<OfficeTaskStatus, string> = {
  queued: '排队中',
  running: '进行中',
  waiting: '等待确认',
  completed: '已完成',
  failed: '失败',
  canceled: '已取消',
};

export const taskStatusTone: Record<OfficeTaskStatus, string> = {
  queued: 'idle',
  running: 'active',
  waiting: 'waiting',
  completed: 'done',
  failed: 'failed',
  canceled: 'canceled',
};

export function isTerminalTask(status: OfficeTaskStatus) {
  return status === 'completed' || status === 'failed' || status === 'canceled';
}

export function formatBytes(size: number) {
  if (!Number.isFinite(size) || size < 0) return '未知';
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

export function formatClock(iso?: string) {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleTimeString('zh-CN', {hour: '2-digit', minute: '2-digit'});
}

export function formatDuration(ms?: number) {
  if (!ms || ms <= 0) return '';
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) return `${seconds} 秒`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes} 分 ${seconds % 60} 秒`;
}
