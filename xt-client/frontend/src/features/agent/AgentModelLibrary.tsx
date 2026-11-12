import {useMemo, useState} from 'react';
import {Check, Cpu, Image, Search, SlidersHorizontal, Sparkles, X, RefreshCw} from 'lucide-react';
import type {AgentModelOption} from '../../lib/api';
import type {AgentModelSettings} from '../../lib/agent-model-settings';
import {ModelPriceBadge} from '../../components/ModelPriceBadge';
import {formatTokens} from '../accounts/modelCatalogUtils';
import {withMissingModels} from './modelOptions';
import './model-library.css';

export function modelName(item: AgentModelOption) {
  const label = item.label || item.upstreamModelId || item.model;
  const suffix = item.sourceLabel ? ` · ${item.sourceLabel}` : '';
  return suffix && label.endsWith(suffix) ? label.slice(0, -suffix.length) : label;
}
export function modelSummary(value?: number) {return value && value > 0 ? formatTokens(value) : '未配置';}

interface Props {
  models: AgentModelOption[];
  chosen: Set<string>;
  loading: boolean;
  error: string;
  onRetry: () => void;
  onAccounts: () => void;
  onToggle: (id: string) => void;
  onAdd: (ids: string[]) => void;
  configFor: (id: string) => AgentModelSettings;
  onSetting: (id: string, key: 'contextWindow' | 'maxOutputTokens', value: string) => void;
  onReasoning: (id: string, value: string) => void;
}

export function AgentModelLibrary({models, chosen, loading, error, onRetry, onAccounts, onToggle, onAdd, configFor, onSetting, onReasoning}: Props) {
  const [query, setQuery] = useState('');
  const [source, setSource] = useState('');
  const [scope, setScope] = useState<'all' | 'chosen'>(() => chosen.size ? 'chosen' : 'all');
  const [capability, setCapability] = useState<'all' | 'images' | 'reasoning'>('all');
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const catalog = useMemo(() => withMissingModels(models, [...chosen]), [models, chosen]);
  const sources = useMemo(() => [...new Map(catalog.map(item => [item.sourceId || 'unassigned', item.sourceLabel || '未分配来源'])).entries()], [catalog]);
  const visible = catalog.filter(item => (!source || (item.sourceId || 'unassigned') === source) && (scope === 'all' || chosen.has(item.model)) && (capability === 'all' || (capability === 'images' ? item.supportsImages : item.supportsReasoning)) && `${item.label} ${item.upstreamModelId || ''} ${item.model} ${item.sourceLabel || ''}`.toLowerCase().includes(query.trim().toLowerCase()));
  const addable = visible.filter(item => !item.unavailable && !chosen.has(item.model));
  const reset = () => {setQuery(''); setSource(''); setCapability('all');};
  return <div className="model-library">
    <div className="model-library-heading"><div><span className="model-library-eyebrow">YOUR MODEL LIBRARY</span><h3>让常用模型触手可及</h3><p>直接勾选，可连续添加；执行参数按需调整。</p></div><button className="model-library-icon-button" type="button" aria-label="刷新模型列表" title="重新读取本地目录" disabled={loading} onClick={onRetry}><RefreshCw className={loading ? 'is-spinning' : ''} size={16}/></button></div>
    <div className="model-library-toolbar">
      <div className="model-library-scope" aria-label="模型范围"><button type="button" aria-pressed={scope === 'chosen'} onClick={() => setScope('chosen')}>常用模型 <b>{chosen.size}</b></button><button type="button" aria-pressed={scope === 'all'} onClick={() => setScope('all')}>全部模型 <b>{catalog.length}</b></button></div>
      <label className="model-library-search"><Search size={16}/><input aria-label="搜索模型" placeholder="搜索名称、来源或模型 ID" value={query} onChange={event => setQuery(event.target.value)}/>{query && <button type="button" onClick={() => setQuery('')} aria-label="清除模型搜索"><X size={14}/></button>}</label>
    </div>
    <div className="model-library-filters"><label><span className="sr-only">筛选模型来源</span><select aria-label="筛选模型来源" value={source} onChange={event => setSource(event.target.value)}><option value="">全部来源 · {sources.length}</option>{sources.map(([id, label]) => <option key={id} value={id}>{label}</option>)}</select></label><div className="model-library-capabilities" aria-label="筛选模型能力">{([['all', '不限能力'], ['images', '图片'], ['reasoning', '推理']] as const).map(([value, label]) => <button type="button" key={value} aria-pressed={capability === value} onClick={() => setCapability(value)}>{label}</button>)}</div></div>
    {loading && <p className="model-library-notice" role="status">正在读取本地模型目录…</p>}
    {error && <div className="model-library-notice is-error" role="alert">目录加载失败：{error}。已有选择会保留。<button type="button" onClick={onRetry}>重试</button></div>}
    <div className="model-library-result-head"><span aria-live="polite">显示 {visible.length} 个模型</span>{scope === 'all' && addable.length > 0 && <button type="button" onClick={() => onAdd(addable.map(item => item.model))}>添加当前结果 · {addable.length}</button>}</div>
    <div className="model-library-grid">
      {visible.map(item => {
        const selected = chosen.has(item.model), open = expanded.has(item.model) && selected;
        const name = modelName(item), config = selected ? configFor(item.model) : {contextWindow:item.contextWindow, maxOutputTokens:item.maxOutputTokens, reasoningEffort:undefined};
        return <article className={`model-library-card${selected ? ' is-selected' : ''}${item.unavailable ? ' is-unavailable' : ''}`} key={item.model}>
          <button type="button" role="checkbox" aria-checked={selected} aria-label={`${name} · ${item.sourceLabel || '未分配来源'}`} disabled={item.unavailable && !selected} className="model-library-choice" onClick={() => onToggle(item.model)}>
            <span className="model-library-glyph"><Cpu size={18}/></span><span className="model-library-copy"><span className="model-library-source">{item.sourceLabel || '未分配来源'}{item.unavailable && <em>不可用</em>}</span><strong>{name}<ModelPriceBadge {...item}/></strong><code title={item.upstreamModelId || item.model}>{item.upstreamModelId || item.model}</code></span><span className="model-library-check">{selected && <Check size={13}/>}</span>
          </button>
          <div className="model-library-card-foot"><span title="目录或 Agent 配置的上下文窗口">上下文 {modelSummary(config.contextWindow)}</span><span className="model-library-feature-icons">{item.supportsImages && <Image size={13} aria-label="支持图片"/>}{item.supportsReasoning && <Sparkles size={13} aria-label="支持推理"/>}</span>{selected && <button type="button" aria-expanded={open} aria-label={`调整 ${name} 参数`} onClick={() => setExpanded(current => {const next = new Set(current); if (next.has(item.model)) next.delete(item.model); else next.add(item.model); return next;})}><SlidersHorizontal size={13}/>参数</button>}</div>
          {selected && <div className={`model-library-parameters${open ? ' is-open' : ''}`} inert={!open} aria-hidden={!open}><div><div className="agent-model-execution">
            <label><span>上下文窗口</span><div><input aria-label={`${name} 上下文窗口`} type="number" min="1024" step="1024" value={config.contextWindow ?? ''} placeholder="未知" onChange={event => onSetting(item.model, 'contextWindow', event.target.value)}/><small>tokens</small></div></label>
            <label><span>最大输出</span><div><input aria-label={`${name} 最大输出`} type="number" min="16" step="256" value={config.maxOutputTokens ?? ''} placeholder="跟随上游" onChange={event => onSetting(item.model, 'maxOutputTokens', event.target.value)}/><small>tokens</small></div></label>
            {item.supportsReasoning && <label><span>思考等级</span><div><select aria-label={`${name} 思考等级`} value={config.reasoningEffort ?? ''} onChange={event => onReasoning(item.model, event.target.value)}><option value="">跟随上游</option>{(item.reasoningEfforts || []).map(value => <option key={value} value={value}>{value}</option>)}</select></div></label>}
          </div></div></div>}
        </article>;
      })}
    </div>
    {!visible.length && !loading && <div className="model-library-empty"><Cpu size={26}/><strong>{!catalog.length ? '还没有可用模型' : scope === 'chosen' && !chosen.size ? '选几款顺手的模型吧' : '没有匹配的模型'}</strong><p>{!catalog.length ? '在模型接口中添加来源并启用模型，随后回到这里选择。' : '可按来源、名称或能力寻找模型，勾选后统一保存。'}</p><div>{!catalog.length ? <button type="button" className="app-modal-button secondary" onClick={onAccounts}>前往模型接口</button> : <button type="button" className="app-modal-button secondary" onClick={() => {reset(); setScope('all');}}>浏览全部模型</button>}</div></div>}
  </div>;
}
