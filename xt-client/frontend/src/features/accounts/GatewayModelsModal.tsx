import {useEffect, useRef, useState} from 'react';
import {Braces, Check, Copy, Cpu, Image, RefreshCw, Search, Sparkles, X} from 'lucide-react';
import {ModalFrame} from '../../components/modal/ModalFrame';
import {Select} from '../../components/select/Select';
import {gatewayModels, exportedModel, type GatewayModel} from '../../lib/gateway-models';
import {copyText} from '../../lib/clipboard';
import {MODEL_CATALOG_CHANGED_EVENT} from '../../lib/model-sources';
import {formatTokens} from './modelCatalogUtils';
import './gateway-models.css';

function tokens(value?: number) {return value && value > 0 ? formatTokens(value) : '未配置';}
export function GatewayModelsModal({onClose}: {onClose: () => void}) {
  const [models, setModels] = useState<GatewayModel[]>([]);
  const [loading, setLoading] = useState(true), [error, setError] = useState('');
  const [query, setQuery] = useState(''), [source, setSource] = useState('');
  const [attempt, setAttempt] = useState(0), [feedback, setFeedback] = useState<{key: string; error?: boolean} | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null), live = useRef(true);
  useEffect(() => {live.current = true; return () => {live.current = false; if (timer.current) clearTimeout(timer.current);};}, []);
  useEffect(() => {
    const controller = new AbortController(); setLoading(true); setError('');
    gatewayModels(controller.signal).then(data => {if (!controller.signal.aborted) setModels(data.models || []);}).catch(reason => {if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : String(reason));}).finally(() => {if (!controller.signal.aborted) setLoading(false);});
    return () => controller.abort();
  }, [attempt]);
  useEffect(() => {const refresh = () => setAttempt(current => current + 1); window.addEventListener(MODEL_CATALOG_CHANGED_EVENT, refresh); return () => window.removeEventListener(MODEL_CATALOG_CHANGED_EVENT, refresh);}, []);
  const sources = [...new Map(models.map(model => [model.source_id, model.source_label])).entries()];
  const visible = models.filter(model => (!source || model.source_id === source) && `${model.id} ${model.label} ${model.source_label} ${model.upstream_model_id}`.toLowerCase().includes(query.trim().toLowerCase()));
  async function copy(key: string, value: string) {
    const ok = await copyText(value); if (!live.current) return;
    if (timer.current) clearTimeout(timer.current);
    setFeedback({key, error:!ok}); timer.current = setTimeout(() => setFeedback(null), 2000);
  }
  const copied = (key: string) => feedback?.key === key && !feedback.error;
  return <ModalFrame title="模型调用清单" description="与 /v1/models 保持一致，包含已启用的手动来源和插件模型。" className="gateway-models-modal" onClose={onClose} footer={<><span className="gateway-models-status" role={feedback?.error ? 'alert' : 'status'}>{feedback?.error ? '复制失败，可选中 ID 手动复制。' : feedback ? '已复制到剪贴板' : '配置为通用 JSON；窗口与能力以来源声明为准。'}</span><button type="button" className="app-modal-button secondary" onClick={onClose}>关闭</button></>}>
    <div className="gateway-models-hero"><span className="gateway-models-hero-icon"><Cpu size={22}/></span><div><strong>对外模型配置，一处复制</strong><p>{models.length} 个已开放模型 <span>·</span> {sources.length} 个来源</p></div><button type="button" className="gateway-models-refresh" disabled={loading} onClick={() => setAttempt(current => current + 1)} aria-label="刷新模型调用清单" title="重新读取本地目录"><RefreshCw size={16} className={loading ? 'is-spinning' : ''}/></button></div>
    <div className="gateway-models-toolbar"><label className="gateway-models-search"><Search size={16}/><input aria-label="搜索模型调用清单" value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索模型、调用 ID 或来源"/>{query && <button type="button" aria-label="清除清单搜索" onClick={() => setQuery('')}><X size={14}/></button>}</label><Select aria-label="筛选调用清单来源" value={source} onChange={setSource} options={[{value: '', label: '全部来源'}, ...sources.map(([id, label]) => ({value: id, label}))]}/></div>
    <div className="gateway-models-bulk"><span aria-live="polite">{loading ? '正在读取目录…' : `当前显示 ${visible.length} 个`}</span><button type="button" disabled={!visible.length || loading || !!error} onClick={() => void copy('ids', visible.map(model => model.id).join('\n'))}>{copied('ids') ? <Check size={14}/> : <Copy size={14}/>}{copied('ids') ? '已复制' : '复制当前 ID 列表'}</button><button type="button" disabled={!visible.length || loading || !!error} onClick={() => void copy('configs', JSON.stringify(visible.map(exportedModel), null, 2))}>{copied('configs') ? <Check size={14}/> : <Braces size={14}/>}{copied('configs') ? '已复制' : '复制当前配置'}</button></div>
    {error && <div className="gateway-models-empty is-error" role="alert"><strong>模型清单读取失败</strong><p>{error}</p><button className="app-modal-button secondary" type="button" onClick={() => setAttempt(current => current + 1)}>重试</button></div>}
    {!error && <div className="gateway-models-list" aria-busy={loading}>
      {visible.map(model => <article className="gateway-model-row" key={model.id}>
        <div className="gateway-model-identity"><span className="gateway-model-source">{model.source_label}</span><h3>{model.label || model.upstream_model_id}</h3><div className="gateway-model-id"><code>{model.id}</code><button type="button" aria-label={`复制模型 ID ${model.id}`} title="复制对外调用 ID" onClick={() => void copy(model.id, model.id)}>{copied(model.id) ? <Check size={15}/> : <Copy size={15}/>}</button></div></div>
        <dl className="gateway-model-metadata"><div><dt>上下文窗口</dt><dd title={model.max_input_tokens ? `${model.max_input_tokens.toLocaleString()} tokens` : '来源未配置'}>{tokens(model.max_input_tokens)}</dd></div><div><dt>最大输出</dt><dd title={model.max_output_tokens ? `${model.max_output_tokens.toLocaleString()} tokens` : '来源未配置'}>{tokens(model.max_output_tokens)}</dd></div>{model.reasoning_efforts?.length ? <div className="gateway-model-efforts"><dt>思考档位</dt><dd>{model.reasoning_efforts.join(' / ')}</dd></div> : null}</dl>
        <div className="gateway-model-actions"><div className="gateway-model-capabilities">{model.supports_images && <span><Image size={12}/>图片</span>}{model.supports_reasoning && <span title={model.reasoning_efforts?.join(' / ')}><Sparkles size={12}/>推理</span>}{!model.supports_images && !model.supports_reasoning && <span>文本</span>}</div><button type="button" onClick={() => void copy(`config:${model.id}`, JSON.stringify(exportedModel(model), null, 2))}>{copied(`config:${model.id}`) ? <Check size={13}/> : <Braces size={13}/>}{copied(`config:${model.id}`) ? '已复制' : '复制配置'}</button></div>
      </article>)}
      {!loading && !visible.length && <div className="gateway-models-empty"><Cpu size={25}/><strong>{models.length ? '没有匹配的模型' : '暂时没有开放的模型'}</strong><p>{models.length ? '试试其他关键词，或查看全部来源。' : '请先添加并启用模型接口，或在来源插件中开放模型。'}</p>{models.length > 0 && <button className="app-modal-button secondary" type="button" onClick={() => {setQuery(''); setSource('');}}>清除筛选</button>}</div>}
    </div>}
  </ModalFrame>;
}
