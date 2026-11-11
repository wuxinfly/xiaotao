import {useState} from 'react';
import type {AgentModelOption} from './api';

export interface AgentModelSettings {
  contextWindow?: number;
  maxOutputTokens?: number;
  reasoningEffort?: string;
}

export interface AgentModelConfig extends AgentModelSettings {
  model: string;
}

export interface AgentDefaultModels {
  title?: string;
  review?: string;
  compaction?: string;
}

export interface AgentModelDefaults {
  title?: AgentModelSettings;
  review?: AgentModelSettings;
  compaction?: AgentModelSettings;
}

export const enabledModelsKey = 'xiaotao.agent.enabledModels';
export const modelLabelsKey = 'xiaotao.agent.modelLabels';
const modelConfigsKey = 'xiaotao.agent.modelConfigs';
const defaultModelsKey = 'xiaotao.agent.defaultModels';
const defaultModelSettingsKey = 'xiaotao.agent.defaultModelSettings';

export function readModelLabels(): Record<string, string> {
  try {
    const value = JSON.parse(localStorage.getItem(modelLabelsKey) || '{}');
    return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
  } catch { return {}; }
}

export function readEnabledModels() {
  try {
    const value = JSON.parse(localStorage.getItem(enabledModelsKey) || '[]');
    return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : [];
  } catch {
    return [];
  }
}

function readModelConfigs(): AgentModelConfig[] {
  try {
    const value = JSON.parse(localStorage.getItem(modelConfigsKey) || '[]');
    if (!Array.isArray(value)) return [];
    return value.filter(item => item && typeof item.model === 'string').map(item => ({
      model: item.model,
      contextWindow: Number(item.contextWindow) > 0 ? Number(item.contextWindow) : undefined,
      maxOutputTokens: Number(item.maxOutputTokens) > 0 ? Number(item.maxOutputTokens) : undefined,
      reasoningEffort: typeof item.reasoningEffort === 'string' ? item.reasoningEffort : undefined,
    }));
  } catch { return []; }
}

function readDefaultModels(): AgentDefaultModels {
  try {
    const value = JSON.parse(localStorage.getItem(defaultModelsKey) || '{}');
    return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
  } catch { return {}; }
}

function readDefaultModelSettings(): AgentModelDefaults {
  try {
    const value = JSON.parse(localStorage.getItem(defaultModelSettingsKey) || '{}');
    return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
  } catch { return {}; }
}

export function modelConfigFor(configs: AgentModelConfig[], model: string, options: AgentModelOption[]): AgentModelConfig | undefined {
  if (!model) return undefined;
  const saved = configs.find(item => item.model === model);
  const option = options.find(item => item.model === model);
  const contextWindow = saved?.contextWindow || option?.contextWindow;
  const maxOutputTokens = saved?.maxOutputTokens || option?.maxOutputTokens;
  if (!contextWindow && !maxOutputTokens && !saved?.reasoningEffort) return undefined;
  return {model, contextWindow, maxOutputTokens, reasoningEffort: saved?.reasoningEffort};
}

export function modelSettingsFor(configs: AgentModelConfig[], model: string, options: AgentModelOption[]) {
  const config = modelConfigFor(configs, model, options);
  if (!config) return undefined;
  return {contextWindow: config.contextWindow, maxOutputTokens: config.maxOutputTokens, reasoningEffort: config.reasoningEffort};
}

export function utilityModelFor(role: keyof AgentDefaultModels, defaults: AgentDefaultModels, settings: AgentModelDefaults, configs: AgentModelConfig[], options: AgentModelOption[]) {
  const model = defaults[role];
  if (!model) return {model: '', settings: undefined as AgentModelSettings | undefined};
  const saved = settings[role];
  const config = modelConfigFor(configs, model, options);
  return {model, settings: config || saved ? {contextWindow: saved?.contextWindow || config?.contextWindow, maxOutputTokens: saved?.maxOutputTokens || config?.maxOutputTokens, reasoningEffort: saved?.reasoningEffort} : undefined};
}

// useAgentModelSettings holds the agent-side model selections (enabled models,
// per-model execution settings and utility model defaults) backed by
// localStorage so they survive reloads.
export function useAgentModelSettings() {
  const [enabledModels, setEnabledModels] = useState<string[]>(readEnabledModels);
  const [modelConfigs, setModelConfigs] = useState<AgentModelConfig[]>(readModelConfigs);
  const [defaultModels, setDefaultModels] = useState<AgentDefaultModels>(readDefaultModels);
  const [defaultModelSettings, setDefaultModelSettings] = useState<AgentModelDefaults>(readDefaultModelSettings);
  function persist(nextModels: string[], nextConfigs: AgentModelConfig[], nextDefaults: AgentDefaultModels, nextDefaultSettings: AgentModelDefaults) {
    setEnabledModels(nextModels);
    setModelConfigs(nextConfigs);
    setDefaultModels(nextDefaults);
    setDefaultModelSettings(nextDefaultSettings);
    localStorage.setItem(enabledModelsKey, JSON.stringify(nextModels));
    localStorage.setItem(modelConfigsKey, JSON.stringify(nextConfigs));
    localStorage.setItem(defaultModelsKey, JSON.stringify(nextDefaults));
    localStorage.setItem(defaultModelSettingsKey, JSON.stringify(nextDefaultSettings));
  }
  return {enabledModels, modelConfigs, defaultModels, defaultModelSettings, persist};
}
