import {useEffect, useId, useRef, useState} from 'react';
import {Check, ChevronDown, Search, X} from 'lucide-react';
import type {AgentModelOption} from '../../lib/api';
import {modelName, modelSummary} from './AgentModelLibrary';

export function UtilityModelPicker({label, value, models, onChange}: {label: string; value: string; models: AgentModelOption[]; onChange: (id: string) => void}) {
  const [open, setOpen] = useState(false), [query, setQuery] = useState('');
  const root = useRef<HTMLDivElement>(null), trigger = useRef<HTMLButtonElement>(null), search = useRef<HTMLInputElement>(null);
  const id = useId();
  const current = models.find(item => item.model === value);
  const visible = models.filter(item => !item.unavailable && `${item.label} ${item.upstreamModelId || ''} ${item.sourceLabel || ''}`.toLowerCase().includes(query.trim().toLowerCase()));
  useEffect(() => {if (open) search.current?.focus();}, [open]);
  useEffect(() => {
    if (!open) return;
    const outside = (event: PointerEvent) => {if (!root.current?.contains(event.target as Node)) setOpen(false);};
    document.addEventListener('pointerdown', outside); return () => document.removeEventListener('pointerdown', outside);
  }, [open]);
  const select = (model: string) => {onChange(model); setOpen(false); trigger.current?.focus();};
  return <div className="utility-model-picker" ref={root} data-escape-layer={open || undefined} onBlur={event => {if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);}} onKeyDown={event => {
    if (event.key === 'Escape' && open) {event.preventDefault(); event.stopPropagation(); setOpen(false); trigger.current?.focus();}
    if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key) && open && event.target !== search.current) {
      const options = [...(root.current?.querySelectorAll<HTMLButtonElement>('[role="option"]') || [])];
      const index = options.indexOf(document.activeElement as HTMLButtonElement);
      const next = event.key === 'Home' ? 0 : event.key === 'End' ? options.length - 1 : (index + (event.key === 'ArrowUp' ? -1 : 1) + options.length) % options.length;
      event.preventDefault(); options[next]?.focus();
    }
  }}>
    <span className="utility-model-label" id={id}>{label}</span>
    <button type="button" className="utility-model-trigger" ref={trigger} role="combobox" aria-label={label} aria-controls={`${id}-list`} aria-expanded={open} aria-haspopup="listbox" onClick={() => {setOpen(!open); setQuery('');}}><span><strong>{value ? current ? `${modelName(current)}${current.unavailable ? '（不可用）' : ''}` : '原模型已不可用' : '跟随会话模型'}</strong><small>{value ? current?.sourceLabel || value : '使用当前对话的模型，无需单独配置'}</small></span><ChevronDown size={16}/></button>
    <div className={`utility-model-options${open ? ' is-open' : ''}`} inert={!open} aria-hidden={!open}><div>
      <label className="model-library-search"><Search size={15}/><input ref={search} value={query} onChange={event => setQuery(event.target.value)} aria-label={`搜索${label}`} placeholder="搜索模型或来源" onKeyDown={event => {if (event.key === 'ArrowDown') {event.preventDefault(); root.current?.querySelector<HTMLButtonElement>('[role="option"]')?.focus();}}}/>{query && <button type="button" aria-label={`清除${label}搜索`} onClick={() => setQuery('')}><X size={13}/></button>}</label>
      <div className="utility-model-list" id={`${id}-list`} role="listbox" aria-labelledby={id}>
        <button type="button" role="option" aria-selected={!value} onClick={() => select('')}><span><strong>跟随会话模型</strong><small>保持与当前对话一致</small></span>{!value && <Check size={15}/>}</button>
        {visible.map(item => <button type="button" role="option" aria-selected={item.model === value} key={item.model} onClick={() => select(item.model)}><span><strong>{modelName(item)}</strong><small>{item.sourceLabel || '未分配来源'} · {modelSummary(item.contextWindow)}</small></span>{item.model === value && <Check size={15}/>}</button>)}
        {!visible.length && <p className="model-library-notice">没有匹配的可用模型</p>}
      </div>
    </div></div>
  </div>;
}
