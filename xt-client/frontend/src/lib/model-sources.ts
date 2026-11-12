import {request} from './http';

export interface SourceModel {
  id: string;
  upstreamModelId: string;
  label: string;
  contextWindow?: number;
  maxOutputTokens?: number;
  supportsImages: boolean;
  supportsReasoning: boolean;
  reasoningEfforts?: string[];
}
export type SourceProtocol = 'openai-chat' | 'openai-responses' | 'anthropic-messages';
export const sourceProtocols: Record<SourceProtocol, string> = {'openai-chat': 'OpenAI Chat Completions', 'openai-responses': 'OpenAI Responses', 'anthropic-messages': 'Anthropic Messages'};

export interface Source {
  id: string;
  label: string;
  kind: 'manual';
  protocol: SourceProtocol;
  baseURL: string;
  enabled: boolean;
  hasAPIKey: boolean;
  models: SourceModel[];
}
export interface SourceInput {
  label: string;
  protocol: SourceProtocol;
  baseURL: string;
  enabled: boolean;
  apiKey?: string;
  models: (Omit<SourceModel, 'id'> & {id?: string})[];
}
export const MODEL_CATALOG_CHANGED_EVENT = 'xiaotao:model-catalog-changed';
export function modelCatalogChanged() { window.dispatchEvent(new Event(MODEL_CATALOG_CHANGED_EVENT)); }
const pathFor = (id: string) => `/api/model-sources/${encodeURIComponent(id)}`;
export const modelSources = {
  list: (signal?: AbortSignal) => request<{sources: Source[]}>('/api/model-sources', {signal}),
  save: (input: SourceInput, id?: string, signal?: AbortSignal) => request<Source>(id ? pathFor(id) : '/api/model-sources', {method: id ? 'PUT' : 'POST', body: JSON.stringify(input), signal}),
  remove: (id: string, signal?: AbortSignal) => request<{ok: true}>(pathFor(id), {method: 'DELETE', signal}),
  test: (id: string, signal?: AbortSignal) => request<{ok: true}>(`${pathFor(id)}/test`, {method: 'POST', signal}),
};
export function sourceInput(source: Source): SourceInput {
  return {label: source.label, protocol: source.protocol, baseURL: source.baseURL, enabled: source.enabled, models: source.models};
}
