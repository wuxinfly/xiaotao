import {useEffect, useState} from 'react';
import {Database, LoaderCircle, RefreshCw, Search} from 'lucide-react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {modelSources, type CatalogModel, type SourceProtocol} from '../../lib/model-sources';

interface Props {
  baseURL: string;
  protocol: SourceProtocol;
  apiKey?: string;
  sourceId?: string;
  existingIDs: ReadonlySet<string>;
  returnFocus: HTMLButtonElement | null;
  onAdd: (models: CatalogModel[]) => void;
  onClose: () => void;
}

type Results = {status: 'loading'} | {status: 'error'; message: string} | {status: 'ready'; models: CatalogModel[]};

export function SourceCatalogPicker({baseURL, protocol, apiKey, sourceId, existingIDs, returnFocus, onAdd, onClose}: Props) {
  const [results, setResults] = useState<Results>({status: 'loading'});
  const [attempt, setAttempt] = useState(0);
  const [query, setQuery] = useState('');
  const [selected, setSelected] = useState(() => new Set<string>());
  useEffect(() => {
    const controller = new AbortController();
    modelSources.discover(baseURL, protocol, apiKey, sourceId, controller.signal).then(result => {
      if (!controller.signal.aborted) setResults({status: 'ready', models: result.models});
    }).catch(reason => {
      if (!controller.signal.aborted) setResults({status: 'error', message: reason instanceof Error ? reason.message : String(reason)});
    });
    return () => controller.abort();
  }, [baseURL, protocol, apiKey, sourceId, attempt]);

  const models = results.status === 'ready' ? results.models : [];
  const term = query.trim().toLowerCase();
  const visible = models.filter(model => model.id.toLowerCase().includes(term) || model.display_name?.toLowerCase().includes(term));
  const chosen = models.filter(model => selected.has(model.id) && !existingIDs.has(model.id));
  const retry = () => {setResults({status: 'loading'}); setAttempt(value => value + 1);};
  function toggle(id: string, checked: boolean) {
    setSelected(current => {
      const next = new Set(current);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  return <ModalFrame title="获取可用模型" description="选择要添加到当前接口的模型。" className="source-catalog-picker"
    stacked returnFocus={returnFocus} onClose={onClose} footer={<>
      <span className="source-picker-count" aria-live="polite">已选 {chosen.length} 项</span>
      <div className="app-modal-actions">
        <button type="button" className="app-modal-button secondary" onClick={onClose}>取消</button>
        <button type="button" className="app-modal-button primary" disabled={!chosen.length} onClick={() => onAdd(chosen)}>添加选中的模型</button>
      </div>
    </>}>
    <div className="source-picker-content" aria-busy={results.status === 'loading'}>
      {results.status === 'loading' && <div className="source-picker-state" role="status"><LoaderCircle size={20} className="source-picker-spinner" aria-hidden="true"/>正在获取模型…</div>}
      {results.status === 'error' && <div className="source-picker-state" role="alert">
        <strong>获取模型失败</strong><span>{results.message}</span>
        <button type="button" className="app-modal-button secondary" onClick={retry}><RefreshCw size={14} aria-hidden="true"/>重试</button>
      </div>}
      {results.status === 'ready' && (models.length ? <>
        <label className="source-picker-search"><Search size={16} aria-hidden="true"/>
          <input type="search" aria-label="搜索模型" placeholder="搜索模型名称或 ID" value={query} onChange={event => setQuery(event.target.value)}/>
        </label>
        <p className="source-picker-count" role="status">{visible.length === models.length ? `${models.length} 个可用模型` : `${visible.length} / ${models.length} 个模型`}</p>
        {visible.length ? <ul className="source-picker-list" aria-label="上游模型">
          {visible.map(model => {
            const added = existingIDs.has(model.id);
            const label = model.display_name?.trim();
            return <li key={model.id}><label className="source-picker-row" data-added={added}>
              <input type="checkbox" disabled={added} checked={added || selected.has(model.id)} onChange={event => toggle(model.id, event.target.checked)}/>
              <span><strong>{label || model.id}</strong>{label && label !== model.id && <code>{model.id}</code>}</span>
              {added && <small>已添加</small>}
            </label></li>;
          })}
        </ul> : <div className="source-picker-state" role="status">没有匹配的模型。</div>}
      </> : <div className="source-picker-state" role="status"><Database size={20} aria-hidden="true"/>上游没有返回模型，可以手动填写模型 ID。</div>)}
    </div>
  </ModalFrame>;
}
