import {useEffect, useRef, useState} from 'react';
import {Plus, Trash2} from 'lucide-react';
import {api, type MCPServer, type MCPSettings as Settings} from '../../lib/api';
import './mcp-settings.css';

type Draft = {id: number; name: string; command: string; args: string; disabled: boolean};
const message = (error: unknown) => error instanceof Error ? error.message : String(error);

export function MCPSettings({active, onBusyChange}: {active: boolean; onBusyChange: (busy: boolean) => void}) {
  const [rows, setRows] = useState<Draft[]>([]);
  const [source, setSource] = useState<Settings['source']>('empty');
  const [loaded, setLoaded] = useState(false), [loading, setLoading] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [error, setError] = useState(''), [notice, setNotice] = useState('');
  const [busy, setBusy] = useState<number | 'save' | null>(null);
  const [results, setResults] = useState<Record<number, {error?: string; tools?: string[]}>>({});
  const serial = useRef(0);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  useEffect(() => {onBusyChange(busy !== null);}, [busy, onBusyChange]);
  useEffect(() => {
    if (!active || loaded) return;
    const request = new AbortController();
    setLoading(true); setError('');
    void api.agentMCP(request.signal).then(settings => {
      setRows(Object.entries(settings.mcpServers || {}).map(([name, server]) => ({id: serial.current++, name, command:server.command, args:(server.args || []).join('\n'), disabled:!!server.disabled})));
      setSource(settings.source); setLoaded(true);
    }).catch(reason => {if (!request.signal.aborted) setError(message(reason));})
      .finally(() => {if (!request.signal.aborted) setLoading(false);});
    return () => request.abort();
  }, [active, loaded, attempt]);

  function update(id: number, patch: Partial<Draft>) {
    setRows(current => current.map(row => row.id === id ? {...row, ...patch} : row));
    setResults(current => {const next = {...current}; delete next[id]; return next;});
    setNotice(''); setError('');
  }
  function add() {
    const base = '新服务';
    let name = base, suffix = 2;
    while (rows.some(row => row.name === name)) name = `${base}-${suffix++}`;
    setRows(current => [...current, {id:serial.current++, name, command:'', args:'', disabled:false}]);
    setNotice('');
  }
  function server(row: Draft): MCPServer {
    return {command:row.command.trim(), args:row.args === '' ? [] : row.args.split('\n').map(arg => arg.replace(/\r$/, '')), disabled:row.disabled};
  }
  async function save() {
    const names = rows.map(row => row.name.trim());
    if (names.some(name => !name) || new Set(names).size !== names.length || rows.some(row => !row.command.trim())) {
      setError('请填写服务名称和启动命令，服务名称不能重复。'); return;
    }
    setBusy('save'); setError(''); setNotice('');
    try {
      await api.saveAgentMCP({mcpServers:Object.fromEntries(rows.map((row, index) => [names[index], server(row)]))});
      setSource('local'); setNotice('已保存，下一次任务生效。');
    } catch (reason) {setError(message(reason));}
    finally {setBusy(null);}
  }
  async function test(row: Draft) {
    setBusy(row.id); setNotice('');
    const request = new AbortController(); controller.current = request;
    const timeout = window.setTimeout(() => request.abort(), 40000);
    try {const result = await api.testAgentMCP(server(row), request.signal); setResults(current => ({...current, [row.id]:{tools:result.tools}}));}
    catch (reason) {setResults(current => ({...current, [row.id]:{error:request.signal.aborted ? '连接测试已取消或超时。' : message(reason)}}));}
    finally {window.clearTimeout(timeout); controller.current = null; setBusy(null);}
  }

  return <div className="mcp-settings">
    <p className="mcp-intro">浏览器工具为内置功能，默认启用，无需添加且不能删除或禁用。可另外连接其他外部工具。保存后从下一次任务生效，正在运行的任务保持原配置。</p>
    <p className="mcp-trust">只添加你信任的服务：保存后的任务和连接测试会以当前用户身份启动命令。Chrome 浏览器工具首次运行会下载 npm 包，需要 Node.js 和 Chrome。</p>
    {source === 'environment' && <p className="mcp-notice">当前来自启动环境变量。保存后使用本环境的独立配置，不修改原配置文件。</p>}
    {loading && <p role="status">正在读取 MCP 配置…</p>}
    {error && <p className="agent-modal-error" role="alert">{error}</p>}
    {!loaded && !loading && error && <button type="button" className="app-modal-button secondary" onClick={() => setAttempt(value => value + 1)}>重试读取</button>}
    {loaded && <>
      <div className="mcp-toolbar"><button type="button" className="app-modal-button secondary" disabled={busy !== null || rows.length >= 33} onClick={() => add()}><Plus aria-hidden="true"/>添加服务</button></div>

      {rows.map(row => <section className="mcp-card" key={row.id} aria-label={`MCP 服务 ${row.name}`}>
        <div className="mcp-card-head"><label className="mcp-enabled"><input type="checkbox" checked={!row.disabled} disabled={busy !== null || row.name === 'chrome-devtools'} onChange={event => update(row.id, {disabled:!event.target.checked})}/>启用服务</label>{row.name !== 'chrome-devtools' && <button type="button" className="app-modal-button quiet" aria-label={`删除服务 ${row.name}`} disabled={busy !== null} onClick={() => {setRows(current => current.filter(item => item.id !== row.id)); setNotice('');}}><Trash2 aria-hidden="true"/>删除</button>}{row.name === 'chrome-devtools' && <span>内置浏览器 · 始终启用</span>}</div>
        <label className="agent-field"><span>服务名称</span><input value={row.name} disabled={busy !== null || row.name === 'chrome-devtools'} onChange={event => update(row.id, {name:event.target.value})} placeholder="例如 chrome-devtools"/></label>
        <label className="agent-field"><span>启动命令</span><input value={row.command} disabled={busy !== null || row.name === 'chrome-devtools'} onChange={event => update(row.id, {command:event.target.value})} placeholder="例如 npx.cmd 或程序完整路径" spellCheck={false}/></label>
        <label className="agent-field"><span>参数（每行一个，不加 shell 引号）</span><textarea value={row.args} disabled={busy !== null || row.name === 'chrome-devtools'} onChange={event => update(row.id, {args:event.target.value})} rows={3} spellCheck={false}/></label>
        <div className="mcp-test"><button type="button" className="app-modal-button secondary" disabled={busy !== null || !row.command.trim()} onClick={() => void test(row)}>{busy === row.id ? '连接中…' : '测试连接'}</button>{busy === row.id && <button type="button" className="app-modal-button quiet" onClick={() => controller.current?.abort()}>取消测试</button>}<span>测试当前草稿，只读取工具列表。</span></div>
        {results[row.id]?.error && <p className="agent-modal-error" role="alert">{results[row.id].error}</p>}
        {results[row.id]?.tools && <div className="mcp-result" role="status"><strong>连接成功 · {results[row.id].tools!.length} 个工具</strong><p>{results[row.id].tools!.join('、') || '服务未提供工具。'}</p></div>}
      </section>)}
      <div className="mcp-save"><span role="status">{notice || '仅支持 stdio；禁用服务不会加载到新任务。'}</span><button type="button" className="app-modal-button primary" disabled={busy !== null} onClick={() => void save()}>{busy === 'save' ? '保存中…' : '保存 MCP 配置'}</button></div>
    </>}
  </div>;
}
