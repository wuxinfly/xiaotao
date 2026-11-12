import {useEffect, useRef, useState} from 'react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {modelCatalogChanged, modelSources, sourceInput, sourceProtocols, type SourceProtocol, type Source, type SourceInput} from '../../lib/model-sources';
import {emptyModel, SourceModelFields} from './SourceModelFields';

export function SourceEditor({source, onSaved, onClose}: {source?: Source; onSaved: (source: Source) => void; onClose: () => void}) {
  const [form, setForm] = useState<SourceInput>(() => source ? sourceInput(source) : {label: '', baseURL: '', apiKey: '', protocol: 'openai-chat', enabled: true, models: [emptyModel()]});
  const [modelKeys, setModelKeys] = useState(() => form.models.map(model => model.id || crypto.randomUUID()));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const request = useRef<AbortController | null>(null);
  const live = useRef(true);
  useEffect(() => {live.current = true; return () => {live.current = false; request.current?.abort();};}, []);
  function close() {request.current?.abort(); live.current = false; onClose();}
  async function save() {
    if (busy || !live.current) return;
    setError('');
    if (!form.label.trim() || !form.baseURL.trim() || (!source && !form.apiKey?.trim()) || form.models.some(model => !model.upstreamModelId.trim())) {setError('请填写来源名称、API URL、API Key 和每个模型的上游 ID。'); return;}
    try {
      const url = new URL(form.baseURL);
      if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) throw new Error();
    } catch {setError('API URL 必须是 http 或 https 地址，不能包含账号、查询参数或片段。'); return;}
    if (new Set(form.models.map(model => model.upstreamModelId.trim())).size !== form.models.length) {setError('同一来源内的上游模型 ID 不能重复。'); return;}
    if (form.models.some(model => [model.contextWindow, model.maxOutputTokens].some(value => value !== undefined && (!Number.isSafeInteger(value) || value <= 0)) || (model.contextWindow && model.maxOutputTokens && model.maxOutputTokens >= model.contextWindow))) {setError('Token 参数须为正整数，最大输出须小于上下文窗口。'); return;}
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
  return <ModalFrame title={source ? '编辑模型接口' : '添加模型接口'} description="选择 API 服务要求的上游格式；本地调用入口支持三种协议。" className="source-editor" onClose={close} footer={<div className="app-modal-actions"><button type="button" className="app-modal-button secondary" onClick={close}>取消</button><button type="submit" form="source-form" className="app-modal-button primary" disabled={busy}>{busy ? '保存中…' : '保存接口'}</button></div>}>
    <form id="source-form" onSubmit={event => {event.preventDefault(); void save();}}>
      <fieldset disabled={busy} className="source-form-fields">
        <div className="source-form-grid">
          <label><span>来源名称</span><input required value={form.label} onChange={event => setForm({...form, label: event.target.value})} autoComplete="off"/></label>
          <label><span>上游格式</span><select aria-label="上游格式" value={form.protocol} onChange={event => setForm({...form, protocol: event.target.value as SourceProtocol})}>{Object.entries(sourceProtocols).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
        </div>
        <label><span>API URL</span><input aria-label="API URL" required type="url" value={form.baseURL} onChange={event => setForm({...form, baseURL: event.target.value})} placeholder="https://api.example.com/v1"/><small>填写实际 API 前缀（如 /v1）或所选协议的完整端点地址，不会自动补 /v1。</small></label>
        <label><span>API Key</span><input aria-label="API Key" type="password" required={!source} value={form.apiKey ?? ''} onChange={event => setForm({...form, apiKey: event.target.value})} autoComplete="new-password" placeholder={source ? '留空保留已保存的密钥' : '输入 API Key'}/>{source && <small>{source.hasAPIKey ? '已保存密钥；不会显示原密钥。' : '尚未保存密钥。'}</small>}</label>
        {form.models.map((model, index) => <SourceModelFields key={modelKeys[index]} model={model} index={index} removable={form.models.length > 1} onChange={next => setForm(current => ({...current, models: current.models.map((item, i) => i === index ? next : item)}))} onRemove={() => {setModelKeys(current => current.filter((_, i) => i !== index)); setForm(current => ({...current, models: current.models.filter((_, i) => i !== index)}));}}/>)}
        <button type="button" className="app-modal-button secondary" onClick={() => {setModelKeys(current => [...current, crypto.randomUUID()]); setForm(current => ({...current, models: [...current.models, emptyModel()]}));}}>添加模型</button>
      </fieldset>
      {error && <p role="alert" className="source-error">{error}</p>}
    </form>
  </ModalFrame>;
}
