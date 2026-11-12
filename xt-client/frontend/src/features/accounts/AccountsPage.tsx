import {useEffect, useRef, useState} from 'react';
import {Plus, RefreshCw, Server, List} from 'lucide-react';
import {ModalPresence} from '../../components/modal/ModalFrame';
import {useConfirm} from '../../components/confirm/ConfirmProvider';
import {modelCatalogChanged, modelSources, sourceInput, sourceProtocols, type Source} from '../../lib/model-sources';
import {LocalGateway} from './LocalGateway';
import {SourceEditor} from './SourceEditor';
import './sources.css';
import {PluginsPanel} from './PluginsPanel';
import {GatewayModelsModal} from './GatewayModelsModal';

export function AccountsPage() {
  const [sources, setSources] = useState<Source[]>([]);
  const [loading, setLoading] = useState(true);
  const [attempt, setAttempt] = useState(0);
  const [error, setError] = useState('');
  const [editor, setEditor] = useState<Source | 'new' | null>(null);
  const [busy, setBusy] = useState('');
  const [catalogOpen, setCatalogOpen] = useState(false);
  const [feedback, setFeedback] = useState<{id: string; text: string; failed?: boolean} | null>(null);
  const live = useRef(true);
  const operation = useRef<AbortController | null>(null);
  const confirm = useConfirm();
  useEffect(() => {live.current = true; return () => {live.current = false; operation.current?.abort();};}, []);
  useEffect(() => {
    const controller = new AbortController(); setLoading(true); setError('');
    modelSources.list(controller.signal).then(data => {if (!controller.signal.aborted) setSources(data.sources ?? []);}).catch(reason => {if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : String(reason));}).finally(() => {if (!controller.signal.aborted) setLoading(false);});
    return () => controller.abort();
  }, [attempt]);
  async function act(source: Source, action: 'toggle' | 'test' | 'delete') {
    if (busy) return;
    if (action === 'delete' && !await confirm({title: `删除“${source.label}”？`, description: '该接口的模型将不再可用；已有会话会保留原模型引用。', confirmLabel: '删除接口'})) return;
    if (!live.current || operation.current) return;
    const controller = new AbortController(); operation.current = controller;
    setBusy(source.id); setFeedback(null);
    try {
      if (action === 'test') {await modelSources.test(source.id, controller.signal); if (live.current) setFeedback({id: source.id, text: '连接测试成功'});}
      else {
        if (action === 'delete') {await modelSources.remove(source.id, controller.signal); if (live.current) setSources(current => current.filter(item => item.id !== source.id));}
        else {const saved = await modelSources.save({...sourceInput(source), enabled: !source.enabled}, source.id, controller.signal); if (live.current) setSources(current => current.map(item => item.id === saved.id ? saved : item));}
        modelCatalogChanged();
      }
    } catch (reason) {if (live.current && !controller.signal.aborted) setFeedback({id: source.id, text: reason instanceof Error ? reason.message : String(reason), failed: true});}
    finally {operation.current = null; if (live.current) setBusy('');}
  }
  return <section className="content-column model-sources-page">
    <header className="source-page-header"><div><h1>模型接口</h1><p>填写自己的 API 地址、密钥和模型，供小涛调用。</p></div><div className="source-actions"><button type="button" className="app-modal-button secondary" disabled={!!editor} onClick={() => setCatalogOpen(true)}><List size={16}/>模型调用清单</button><button type="button" className="app-modal-button secondary" disabled={loading || !!busy || !!editor} onClick={() => setAttempt(value => value + 1)} aria-label="重新加载接口"><RefreshCw size={16}/></button><button type="button" className="app-modal-button primary" disabled={!!busy || loading} onClick={() => setEditor('new')}><Plus size={16}/>添加接口</button></div></header>
    {loading && <p role="status">正在读取本地模型接口…</p>}
    {error && <div className="source-error" role="alert">加载失败：{error}<button type="button" onClick={() => setAttempt(value => value + 1)}>重试</button></div>}
    {!loading && !error && !sources.length && <div className="source-empty"><Server size={32}/><h2>添加第一个模型接口</h2><p>准备好 API URL、API Key 和上游模型 ID，按服务要求选择上游格式。</p><button type="button" className="app-modal-button primary" onClick={() => setEditor('new')}>添加接口</button></div>}
    <div className="source-list">{sources.map(source => <article className={'source-card' + (source.enabled ? '' : ' is-disabled')} key={source.id} aria-label={source.label}>
      <header><div><h2>{source.label}</h2><code>{source.baseURL}</code><p>{sourceProtocols[source.protocol]} · {source.hasAPIKey ? '密钥已保存' : '未配置密钥'} · {source.enabled ? '已启用' : '已停用'}</p></div><label className="source-toggle"><input type="checkbox" checked={source.enabled} disabled={!!busy || loading} onChange={() => void act(source, 'toggle')} aria-label={`启用 ${source.label}`}/>启用</label></header>
      <ul className="source-model-list">{source.models.map(model => <li key={model.id}><div><strong>{model.label || model.upstreamModelId}</strong><code>{model.upstreamModelId}</code></div><small>{model.contextWindow ? `上下文 ${model.contextWindow.toLocaleString()}` : '上下文未声明'}{model.maxOutputTokens ? ` · 输出 ${model.maxOutputTokens.toLocaleString()}` : ''}{model.supportsImages ? ' · 图片' : ''}{model.supportsReasoning ? ` · 推理${model.reasoningEfforts?.length ? `（${model.reasoningEfforts.join(' / ')}）` : ''}` : ''}</small></li>)}</ul>
      <footer><span role={feedback?.failed ? 'alert' : 'status'} className={feedback?.failed ? 'source-error' : ''}>{busy === source.id ? '处理中…' : feedback?.id === source.id ? feedback.text : `${source.models.length} 个模型`}</span><div className="source-actions"><button type="button" className="app-modal-button secondary" disabled={!!busy || loading} onClick={() => setEditor(source)}>编辑</button><button type="button" className="app-modal-button secondary" disabled={!!busy || loading} onClick={() => void act(source, 'test')}>测试连接</button><button type="button" className="app-modal-button secondary" disabled={!!busy || loading} onClick={() => void act(source, 'delete')}>删除</button></div></footer>
    </article>)}</div>
    <PluginsPanel/>
    <LocalGateway/>
    <ModalPresence>{catalogOpen && <GatewayModelsModal onClose={() => setCatalogOpen(false)}/>}</ModalPresence>
    <ModalPresence>{editor && <SourceEditor key={editor === 'new' ? 'new' : editor.id} source={editor === 'new' ? undefined : editor} onClose={() => setEditor(null)} onSaved={saved => {setSources(current => current.some(item => item.id === saved.id) ? current.map(item => item.id === saved.id ? saved : item) : [...current, saved]); setEditor(null);}}/>}</ModalPresence>
  </section>;
}
