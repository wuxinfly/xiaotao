import './model-options.css';
import {useEffect} from 'react';
import type {AgentModelOption} from '../../lib/api';
import {MODEL_CATALOG_CHANGED_EVENT} from '../../lib/model-sources';

export function withMissingModels(models: AgentModelOption[], ids: string[]): AgentModelOption[] {
  const missing = ids.filter((id, index) => id && ids.indexOf(id) === index && !models.some(item => item.model === id));
  return [...models, ...missing.map(model => ({model, label: model, unavailable: true}))];
}
export function mergeModelCatalog(models: AgentModelOption[], selected: string[], previous: AgentModelOption[]) {
  return withMissingModels(models, selected).map(item => item.unavailable ? {...item, label: previous.find(old => old.model === item.model)?.label || item.label} : item);
}
export function pickerModels(models: AgentModelOption[], selected: string[], current: string, active?: string) {
  return withMissingModels(models.filter(item => selected.includes(item.model) || item.model === current || item.model === active), [current, active || '']);
}
export function modelPickerLabel(models: AgentModelOption[], id: string) {
  const option = models.find(item => item.model === id);
  return option ? `${option.label}${option.unavailable ? '（不可用）' : ''}` : '先在设置中选择模型';
}
export function useModelCatalogChanged(onChange: () => void) {
  useEffect(() => {window.addEventListener(MODEL_CATALOG_CHANGED_EVENT, onChange); return () => window.removeEventListener(MODEL_CATALOG_CHANGED_EVENT, onChange);}, [onChange]);
}
export function ModelPickerOptions({models, value, onSelect}: {models: AgentModelOption[]; value: string; onSelect: (model: string) => void}) {
  const groups = new Map<string, AgentModelOption[]>();
  for (const item of models) {const key = item.sourceId || 'unassigned'; groups.set(key, [...(groups.get(key) || []), item]);}
  return <>{Array.from(groups, ([id, items]) => <div role="group" aria-label={items[0].sourceLabel || '未分配来源'} key={id} className="agent-model-source-group">
    <small>{items[0].sourceLabel || '未分配来源'}</small>
    {items.map(item => <button type="button" role="menuitemradio" aria-checked={item.model === value} disabled={item.unavailable} key={item.model} onClick={() => onSelect(item.model)}><span>{item.label}{item.unavailable ? '（不可用）' : ''}</span>{item.model === value ? '✓' : ''}</button>)}
  </div>)}</>;
}
