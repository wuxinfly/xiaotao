import {useState} from 'react';
import {Check, ChevronDown, Layers, Plus, RefreshCw, Search, X} from 'lucide-react';
import {ModelPriceBadge} from '../../components/ModelPriceBadge';

export type PluginModel = {key: string; label: string; upstreamId?: string; priceFactor?: number; isFree?: boolean};
export type ModelSettings = {selectedKeys: string[]; customIds: string[] | null};
const previewSize = 12;

type CatalogEntry = PluginModel & {custom?: boolean};
function entries(models: PluginModel[], customIds: string[]): CatalogEntry[] {
  return [...models, ...customIds.map(id => ({key: `manual:${id}`, label: id, upstreamId: id, custom: true}))];
}

export function PluginCatalog({models, settings, busy, canRefresh, onRefresh, onSave}: {
  models: PluginModel[]; settings?: ModelSettings; busy: boolean; canRefresh: boolean; onRefresh: () => void; onSave: (settings: ModelSettings) => Promise<boolean>;
}) {
  const [query, setQuery] = useState('');
  const [expanded, setExpanded] = useState(false);
  const [selectedOnly, setSelectedOnly] = useState(false);
  const [modelId, setModelId] = useState('');
  const [draft, setDraft] = useState<ModelSettings | null>(null);
  const current = draft ?? settings ?? {selectedKeys: models.map(model => model.key), customIds: []};
  const customIds = current.customIds ?? [];
  const all = entries(models, customIds);
  const selected = new Set(current.selectedKeys);
  const count = all.filter(model => selected.has(model.key)).length;
  const term = query.trim().toLowerCase();
  const matching = all.filter(model => (!selectedOnly || selected.has(model.key)) &&
    (!term || [model.label, model.upstreamId, model.key].some(value => value?.toLowerCase().includes(term))));
  const visible = expanded ? matching : matching.slice(0, previewSize);

  function toggle(key: string) {
    if (selected.has(key)) selected.delete(key);
    else selected.add(key);
    setDraft({selectedKeys: [...selected], customIds});
  }
  function addModel() {
    const id = modelId.trim();
    if (!id) return;
    const existing = all.find(model => model.upstreamId === id);
    setDraft({selectedKeys: [...new Set([...selected, existing?.key ?? `manual:${id}`])], customIds: existing ? customIds : [...customIds, id]});
    setModelId('');
    setQuery(id);
  }
  function removeModel(id: string) {
    selected.delete(`manual:${id}`);
    setDraft({selectedKeys: [...selected], customIds: customIds.filter(value => value !== id)});
  }
  async function save() {
    if (await onSave({selectedKeys: all.filter(model => selected.has(model.key)).map(model => model.key), customIds})) setDraft(null);
  }
  return <div className="plugin-catalog">
    <div className="plugin-catalog-heading">
      <div className="plugin-service-title"><Layers size={17}/><h4>模型</h4><span className="plugin-count">已选 {count} / {all.length}</span></div>
      {canRefresh && <button type="button" className="app-modal-button secondary" disabled={busy} onClick={onRefresh}><RefreshCw size={14}/>刷新目录</button>}
    </div>
    <div className="plugin-model-toolbar">
      <div className="plugin-model-views" role="group" aria-label="模型视图">
        <button type="button" aria-pressed={!selectedOnly} onClick={() => setSelectedOnly(false)}>供应商目录</button>
        <button type="button" aria-pressed={selectedOnly} onClick={() => setSelectedOnly(true)}>仅显示已选</button>
      </div>
      <label className="plugin-search"><Search size={15} aria-hidden="true"/>
        <input aria-label="搜索模型" type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="按名称或 ID 搜索"/>
      </label>
      <div className="plugin-service-actions">
        <button type="button" className="app-modal-button secondary" disabled={busy || !all.length} onClick={() => setDraft({selectedKeys: all.map(model => model.key), customIds})}>全选</button>
        <button type="button" className="app-modal-button secondary" disabled={busy || !count} onClick={() => setDraft({selectedKeys: [], customIds})}>全不选</button>
      </div>
    </div>
    {visible.length ? <ul className="plugin-models">
      {visible.map(model => <li key={model.key} className={selected.has(model.key) ? 'is-selected' : ''}>
        <button type="button" className="plugin-model-chip" aria-pressed={selected.has(model.key)} title={model.upstreamId || model.key} disabled={busy} onClick={() => toggle(model.key)}>
          {selected.has(model.key) && <Check size={13} aria-hidden="true"/>}{model.label || model.key}
          <ModelPriceBadge priceFactor={model.priceFactor} isFree={model.isFree}/>
        </button>
        {'custom' in model && <button type="button" className="plugin-remove-model" aria-label={`移除 ${model.label}`} disabled={busy} onClick={() => removeModel(model.label)}><X size={13}/></button>}
      </li>)}
    </ul> : <p className="plugin-catalog-empty">{term ? '没有匹配的模型。' : selectedOnly ? '还没有选中模型。' : '暂无目录。可刷新目录或手动添加模型 ID。'}</p>}
    {matching.length > previewSize && <button type="button" className="plugin-more" aria-expanded={expanded} onClick={() => setExpanded(!expanded)}>
      {expanded ? '收起列表' : `显示其余 ${matching.length - previewSize} 个`}<ChevronDown className="plugin-chevron" size={14}/>
    </button>}
    <form className="plugin-add-model" onSubmit={event => {event.preventDefault(); addModel();}}>
      <input aria-label="上游模型 ID" placeholder="输入上游模型 ID" value={modelId} onChange={event => setModelId(event.target.value)} maxLength={512} disabled={busy}/>
      <button className="app-modal-button secondary" disabled={busy || !modelId.trim()}><Plus size={14}/>添加模型</button>
    </form>
    <div className="plugin-model-save">
      <span>{draft ? '有未保存的更改' : '已选模型可在 Agent 和本地网关使用。'}</span>
      {draft && <button type="button" className="app-modal-button secondary" disabled={busy} onClick={() => setDraft(null)}>取消更改</button>}
      <button type="button" className="app-modal-button primary" disabled={busy || !draft} onClick={() => void save()}>保存模型</button>
    </div>
  </div>;
}
