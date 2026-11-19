import {useEffect, useRef, useState} from 'react';
import {Eye, EyeOff} from 'lucide-react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {Select} from '../../components/select/Select';
import {modelCatalogChanged, modelSources, sourceInput, sourceProtocols, type CatalogModel, type SourceProtocol, type Source, type SourceInput} from '../../lib/model-sources';
import {emptyModel, SourceModelFields} from './SourceModelFields';

const protocolPaths: Record<SourceProtocol, string> = {'openai-chat': '/chat/completions', 'openai-responses': '/responses', 'anthropic-messages': '/messages'};

function normalizeBaseURL(raw: string, protocol: SourceProtocol): string | null {
  if (/\s/.test(raw) || raw.includes('?') || raw.includes('#')) return null;
  try {
    const url = new URL(raw);
    if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) return null;
    const path = protocolPaths[protocol];
    const trimmed = url.pathname.replace(/\/+$/, '');
    url.pathname = trimmed.endsWith(path) ? trimmed.slice(0, -path.length) : trimmed;
    const base = url.toString().replace(/\/+$/, '');
    return `${base}${path}`;
  } catch {return null;}
}

function tokenIssue(model: SourceInput['models'][number]) {
  if (model.contextWindow !== undefined && (!Number.isSafeInteger(model.contextWindow) || model.contextWindow < 1024)) return '上下文窗口至少为 1024 tokens';
  if (model.maxOutputTokens !== undefined && (!Number.isSafeInteger(model.maxOutputTokens) || model.maxOutputTokens < 16)) return '最大输出至少为 16 tokens';
  if (model.contextWindow && model.maxOutputTokens && model.maxOutputTokens >= model.contextWindow) return '最大输出须小于上下文窗口';
  return '';
}

export function SourceEditor({source, onSaved, onClose}: {source?: Source; onSaved: (source: Source) => void; onClose: () => void}) {
  const [form, setForm] = useState<SourceInput>(() => source ? sourceInput(source) : {label: '', baseURL: '', apiKey: '', protocol: 'openai-chat', enabled: true, models: [emptyModel()]});
  const [modelKeys, setModelKeys] = useState(() => form.models.map(model => model.id || crypto.randomUUID()));
  const [busy, setBusy] = useState(false);
  const [discovering, setDiscovering] = useState(false);
  const [catalog, setCatalog] = useState<CatalogModel[] | null>(null);
  const [selected, setSelected] = useState(() => new Set<string>());
  const [catalogError, setCatalogError] = useState('');
  const [showAPIKey, setShowAPIKey] = useState(false);
  const [error, setError] = useState('');
  const request = useRef<AbortController | null>(null);
  const live = useRef(true);
  useEffect(() => {live.current = true; return () => {live.current = false; request.current?.abort();};}, []);
  function close() {request.current?.abort(); live.current = false; onClose();}
  function updateConnection(change: Partial<Pick<SourceInput, 'baseURL' | 'protocol' | 'apiKey'>>) {
    setForm(current => ({...current, ...change}));
    setCatalog(null);
    setSelected(new Set());
    setCatalogError('');
  }

  const endpointURL = normalizeBaseURL(form.baseURL, form.protocol);
  const canDiscover = Boolean(endpointURL && (form.apiKey?.trim() || source?.hasAPIKey));

  async function discover() {
    if (busy || discovering || !live.current || !canDiscover) return;
    setDiscovering(true);
    setCatalog(null);
    setSelected(new Set());
    setCatalogError('');
    const controller = new AbortController();
    request.current = controller;
    try {
      const result = await modelSources.discover(form.baseURL, form.protocol, form.apiKey, source?.id, controller.signal);
      if (live.current) setCatalog(result.models);
    } catch (reason) {
      if (live.current && !controller.signal.aborted) setCatalogError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      if (live.current) setDiscovering(false);
    }
  }

  const existingIDs = new Set(form.models.map(model => model.upstreamModelId.trim()));
  const selectedModels = catalog?.filter(model => selected.has(model.id) && !existingIDs.has(model.id)) ?? [];
  function addSelected() {
    if (!selectedModels.length) return;
    const models = selectedModels.map(model => ({...emptyModel(), upstreamModelId: model.id, label: model.display_name?.trim() || model.id}));
    const firstModel = form.models[0];
    const replacePlaceholder = !source && form.models.length === 1 && !firstModel.upstreamModelId && !firstModel.label &&
      firstModel.contextWindow === undefined && firstModel.maxOutputTokens === undefined &&
      !firstModel.supportsImages && !firstModel.supportsReasoning && !firstModel.reasoningEfforts?.length;
    const keys = models.map(() => crypto.randomUUID());
    setForm(current => ({...current, models: replacePlaceholder ? models : [...current.models, ...models]}));
    setModelKeys(current => replacePlaceholder ? keys : [...current, ...keys]);
    setSelected(new Set());
  }

  async function save() {
    if (busy || discovering || !live.current) return;
    setError('');
    if (!form.label.trim() || !form.baseURL.trim() || (!source && !form.apiKey?.trim()) || form.models.some(model => !model.upstreamModelId.trim())) {setError('请填写来源名称、API URL、API Key 和每个模型的上游 ID。'); return;}
    if (!normalizeBaseURL(form.baseURL, form.protocol)) {setError('API URL 必须是 http 或 https 地址，不能包含账号、查询参数或片段。'); return;}
    if (new Set(form.models.map(model => model.upstreamModelId.trim())).size !== form.models.length) {setError('同一来源内的上游模型 ID 不能重复。'); return;}
    const tokenIssueIndex = form.models.findIndex(tokenIssue);
    if (tokenIssueIndex >= 0) {setError(`模型 ${tokenIssueIndex + 1}：${tokenIssue(form.models[tokenIssueIndex])}。`); return;}
    const input: SourceInput = {...form, label: form.label.trim(), baseURL: form.baseURL.trim(), models: form.models.map(model => ({...model, upstreamModelId: model.upstreamModelId.trim(), label: model.label.trim() || model.upstreamModelId.trim(), reasoningEfforts: model.supportsReasoning ? Array.from(new Set(model.reasoningEfforts?.map(effort => effort.trim()).filter(Boolean))) : undefined}))};
    if (!input.apiKey?.trim()) delete input.apiKey;
    setBusy(true);
    const controller = new AbortController(); request.current = controller;
    try {
      const saved = await modelSources.save(input, source?.id, controller.signal);
      modelCatalogChanged();
      if (live.current) onSaved(saved);
    } catch (reason) {if (live.current && !controller.signal.aborted) setError(reason instanceof Error ? reason.message : String(reason));}
    finally {if (live.current) setBusy(false);}
  }
  return <ModalFrame title={source ? '编辑模型接口' : '添加模型接口'} description="选择 API 服务要求的上游格式；本地调用入口支持三种协议。" className="source-editor" onClose={close} footer={<div className="app-modal-actions"><button type="button" className="app-modal-button secondary" onClick={close}>取消</button><button type="submit" form="source-form" className="app-modal-button primary" disabled={busy || discovering}>{busy ? '保存中…' : '保存接口'}</button></div>}>
    <form id="source-form" noValidate onSubmit={event => {event.preventDefault(); void save();}}>
      <fieldset disabled={busy || discovering} className="source-form-fields">
        <div className="source-form-grid">
          <label><span>来源名称 <em aria-hidden="true">必填</em></span><input aria-label="来源名称" required value={form.label} onChange={event => setForm({...form, label: event.target.value})} autoComplete="off"/></label>
          <label><span>上游格式</span><Select aria-label="上游格式" value={form.protocol} onChange={next => updateConnection({protocol: next as SourceProtocol})} options={Object.entries(sourceProtocols).map(([value, label]) => ({value, label}))}/></label>
        </div>
        <label><span>API URL <em aria-hidden="true">必填</em></span><input aria-label="API URL" required type="url" value={form.baseURL} onChange={event => updateConnection({baseURL: event.target.value})} placeholder="https://api.example.com/v1"/><small>填写实际 API 前缀（如 /v1）或所选协议的完整端点地址，不会自动补 /v1。</small>{endpointURL && <small className="source-url-preview">将请求：<code>{endpointURL}</code></small>}</label>
        <label><span>API Key <em aria-hidden="true">{source ? "选填" : "必填"}</em></span><div className="source-api-key"><input aria-label="API Key" type={showAPIKey ? 'text' : 'password'} required={!source} value={form.apiKey ?? ''} onChange={event => updateConnection({apiKey: event.target.value})} autoComplete="new-password" placeholder={source ? '留空保留已保存的密钥' : '输入 API Key'}/><button type="button" className="source-reveal-key" onClick={() => setShowAPIKey(value => !value)} aria-label={showAPIKey ? '隐藏 API Key' : '显示 API Key'} title={showAPIKey ? '隐藏 API Key' : '显示 API Key'}>{showAPIKey ? <EyeOff size={15}/> : <Eye size={15}/>}</button></div>{source && <small>{source.hasAPIKey ? '已保存密钥；留空保留，不会显示原密钥。' : '尚未保存密钥。'}</small>}</label>
        <section className="source-discovery" aria-label="获取可用模型" aria-busy={discovering}>
          <button type="button" className="app-modal-button secondary" disabled={!canDiscover} onClick={() => void discover()}>
            {discovering ? '获取中…' : '获取可用模型'}
          </button>
          {catalogError && <p role="alert" className="source-error">{catalogError}</p>}
          {catalog?.length === 0 && <p role="status">上游没有返回可用模型，可以手动填写模型 ID。</p>}
          {!!catalog?.length && <>
            <ul className="source-discovery-list" aria-label="上游模型" tabIndex={0}>
              {catalog.map(model => <li key={model.id}><label>
                <input type="checkbox" disabled={existingIDs.has(model.id)} checked={existingIDs.has(model.id) || selected.has(model.id)}
                  onChange={event => {
                    const next = new Set(selected);
                    if (event.target.checked) next.add(model.id);
                    else next.delete(model.id);
                    setSelected(next);
                  }}/>
                <span>{model.display_name && model.display_name !== model.id ? `${model.display_name} (${model.id})` : model.id}</span>
              </label></li>)}
            </ul>
            <button type="button" className="app-modal-button secondary" disabled={!selectedModels.length} onClick={addSelected}>
              {selectedModels.length ? `添加选中的 ${selectedModels.length} 个模型` : '添加选中的模型'}
            </button>
          </>}
        </section>
        {form.models.map((model, index) => <SourceModelFields key={modelKeys[index]} model={model} index={index} removable={form.models.length > 1} onChange={next => setForm(current => ({...current, models: current.models.map((item, i) => i === index ? next : item)}))} onRemove={() => {setModelKeys(current => current.filter((_, i) => i !== index)); setForm(current => ({...current, models: current.models.filter((_, i) => i !== index)}));}}/>)}
        <button type="button" className="app-modal-button secondary" onClick={() => {setModelKeys(current => [...current, crypto.randomUUID()]); setForm(current => ({...current, models: [...current.models, emptyModel()]}));}}>添加模型</button>
      </fieldset>
      {error && <p role="alert" className="source-error">{error}</p>}
    </form>
  </ModalFrame>;
}
