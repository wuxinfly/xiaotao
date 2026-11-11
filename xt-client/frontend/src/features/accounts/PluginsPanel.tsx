import {useEffect, useRef, useState} from 'react';
import {request} from '../../lib/http';
import {modelCatalogChanged} from '../../lib/model-sources';

type Plugin = {id: string; enabled: boolean; manifest: {name: string; version: string; entrypoint: string}; models: {key: string; label: string}[]};
export function PluginsPanel() {
  const [legacy, setLegacy] = useState<{id: string; name: string; uid: string}[]>([]);
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [path, setPath] = useState('');
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [result, setResult] = useState('');
  const controller = useRef<AbortController | null>(null);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    const abort = new AbortController();
    request<{plugins: Plugin[]}>('/api/source-plugins', {signal: abort.signal}).then(data => setPlugins(data.plugins ?? [])).catch(error => {if (!abort.signal.aborted) setMessage(String(error));});
    request<{accounts: {id: string; name: string; uid: string}[]}>('/api/source-plugins/legacy-trae', {signal: abort.signal}).then(data => setLegacy(data.accounts ?? [])).catch(() => {});
    return () => {mounted.current = false; abort.abort(); controller.current?.abort();};
  }, []);
  async function run(url: string, body: unknown) {
    if (controller.current) return;
    const abort = new AbortController(); controller.current = abort;
    setBusy(true); setMessage(''); setResult('');
    try {
      const response = await request<{value?: unknown}>(url, {method: 'POST', body: JSON.stringify(body), signal: abort.signal});
      const data = await request<{plugins: Plugin[]}>('/api/source-plugins', {signal: abort.signal});
      if (mounted.current) {setPlugins(data.plugins ?? []); setToken(''); setMessage('操作成功'); if (response.value) setResult(JSON.stringify(response.value, null, 2)); modelCatalogChanged();}
    } catch (error) {if (mounted.current && !abort.signal.aborted) setMessage(String(error));}
    finally {controller.current = null; if (mounted.current) setBusy(false);}
  }
  return <section className="source-card" aria-label="模型来源插件">
    <h2>模型来源插件</h2>
    <p>安装本地插件包后，启用并导入 Trae 国内版刷新令牌，再刷新模型列表。</p>
    <p>插件将以当前用户权限运行。请核对所选目录中的 manifest.json、插件身份和入口，只安装可信代码。</p>
    <form onSubmit={event => {event.preventDefault(); void run('/api/source-plugins/install', {path});}}>
      <label>本地插件包目录<input value={path} onChange={event => setPath(event.target.value)} placeholder="完整目录路径，包含 manifest.json" required disabled={busy}/></label>
      <button className="app-modal-button secondary" disabled={busy || !path.trim()}>安装包（暂不执行）</button>
    </form>
    {plugins.map(plugin => <article key={plugin.id}>
      <h3>{plugin.manifest.name} · {plugin.manifest.version}</h3>
      <p><code>{plugin.id}</code> · 将运行 <code>{plugin.manifest.entrypoint}</code> · {plugin.models?.length ?? 0} 个模型</p>
      <button className="app-modal-button secondary" disabled={busy} onClick={() => void run(`/api/source-plugins/${plugin.id}/enabled`, {enabled: !plugin.enabled})}>{plugin.enabled ? '停用' : '启用'}</button>
      {plugin.enabled && <>
        {plugin.id === 'xiaotao.trae' && legacy.length > 0 && <div><p>从当前环境旧账号导入（替换插件当前账号，保留旧账号和历史会话）：</p>{legacy.map(account => <button className="app-modal-button secondary" key={account.id} disabled={busy} onClick={() => void run('/api/source-plugins/legacy-trae', {id: account.id})}>导入 {account.name || account.uid || account.id}</button>)}</div>}
        <form onSubmit={event => {event.preventDefault(); void run(`/api/source-plugins/${plugin.id}/action`, {method: 'login/import', input: {refreshToken: token}});}}>
          <label>Trae 刷新令牌<input type="password" autoComplete="off" value={token} onChange={event => setToken(event.target.value)} required disabled={busy}/></label>
          <button className="app-modal-button secondary" disabled={busy || !token}>导入登录</button>
        </form>
        <div className="source-actions">{([['models/refresh', '刷新模型'], ['quota/get', '查询配额'], ['checkin/status', '签到状态'], ['checkin/claim', '签到']] as const).map(([method, label]) => <button className="app-modal-button secondary" key={method} disabled={busy} onClick={() => void run(`/api/source-plugins/${plugin.id}/action`, {method})}>{label}</button>)}</div>
      </>}
    </article>)}
    <p role="status">{busy ? '处理中…' : message}</p>
    {result && <pre style={{maxHeight: 240, overflow: 'auto'}}>{result}</pre>}
  </section>;
}
