import {request} from './http';

export type WorkspaceEntry = {name: string; path: string; dir: boolean; size: number; modified: string};
export type WorkspaceListing = {root: string; path: string; parent: string; truncated: boolean; entries: WorkspaceEntry[]};
export type WorkspaceFileKind = 'text' | 'image' | 'binary' | 'too-large';
export type WorkspaceFile = {
  path: string;
  name: string;
  size: number;
  modified: string;
  kind: WorkspaceFileKind;
  truncated: boolean;
  content?: string;
};

function query(projectPath: string, path: string, extra?: Record<string, string>) {
  const params = new URLSearchParams({projectPath, path, ...extra});
  return params.toString();
}

export const workspaceApi = {
  list: (projectPath: string, path: string, signal?: AbortSignal) =>
    request<WorkspaceListing>(`/api/workspace/files?${query(projectPath, path)}`, {signal}),
  file: (projectPath: string, path: string, signal?: AbortSignal) =>
    request<WorkspaceFile>(`/api/workspace/file?${query(projectPath, path)}`, {signal}),
  rawUrl: (projectPath: string, path: string, version?: string) =>
    `/api/workspace/file/raw?${query(projectPath, path, version ? {v: version} : undefined)}`,
  openUrl: (url: string) => request<{ok: boolean}>('/api/system/open-url', {method: 'POST', body: JSON.stringify({url})}),
};
