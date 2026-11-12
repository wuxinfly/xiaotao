import {useEffect, useRef, useState} from 'react';
import {Box, FolderPlus} from 'lucide-react';
import {ApiError, request} from '../../lib/http';
import {modelCatalogChanged} from '../../lib/model-sources';
import {PluginAdmin} from './PluginAdmin';
import {PluginCatalog, type ModelSettings, type PluginModel} from './PluginCatalog';
import {PluginAccounts, type LegacyAccount, type PluginAccount} from './PluginAccounts';
import {PluginIndex, type IndexItem} from './PluginIndex';
import {PluginPackages, type LocalPackage} from './PluginPackages';
import type {PluginDisplay} from './PluginQuota';
import {PluginSettings} from './PluginSettings';
import './plugins.css';

type Plugin = {
  id: string; enabled: boolean; manifest: {name: string; version: string; entrypoint: string; capabilities?: string[]; settings?: SettingsField[]};
  models: PluginModel[]; modelSettings?: ModelSettings; accounts?: PluginAccount[]; settingValues?: Record<string, string>;
  packageHash?: string; diagnostics?: boolean; diagnosticsPath?: string;
  previous?: {manifest: {version: string}; packageHash?: string};
};
type Result = {method: string; value: unknown; display?: PluginDisplay};
type SettingsField = {key: string; label: string; description?: string; placeholder?: string; default?: string; options?: string[]};

function ResultView({result}: {result: Result}) {
  if (!result.value || typeof result.value !== 'object') return null;
  const value = result.value as Record<string, unknown>;
  if (result.method.startsWith('checkin/')) {
    const text = value.supported === false ? '当前账号不支持签到' : value.active === false ? '签到活动未开启' : value.checkedIn ? '今日已签到' : value.ok === false ? '签到未成功' : '今日尚未签到';
    return <div className="plugin-result"><strong>签到</strong><span>{text}{typeof value.dailyCredit === 'number' && value.dailyCredit > 0 ? ` · 每日 ${value.dailyCredit} 积分` : ''}{typeof value.extraCredits === 'number' && value.extraCredits > 0 ? ` · 获得 ${value.extraCredits} 积分` : ''}</span></div>;
  }
  return null;
}

export function PluginsPanel() {
  const [legacy, setLegacy] = useState<LegacyAccount[]>([]);
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [packages, setPackages] = useState<LocalPackage[]>([]);
  const [index, setIndex] = useState<IndexItem[]>([]);
  const [installOpen, setInstallOpen] = useState(false);
  const [path, setPath] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [failed, setFailed] = useState(false);
  const [results, setResults] = useState<Record<string, Result>>({});
  const [authorizing, setAuthorizing] = useState('');
  const [retryLogin, setRetryLogin] = useState('');
  const controller = useRef<AbortController | null>(null);
  const mounted = useRef(true);
  function loadPackages(signal?: AbortSignal) {
    return request<{packages: LocalPackage[]}>('/api/source-plugins/packages', {signal}).then(data => {
      if (mounted.current) setPackages(data.packages ?? []);
    }).catch(() => {});
  }
  function loadIndex(signal?: AbortSignal) {
    return request<{plugins: IndexItem[]}>('/api/source-plugins/index', {signal}).then(data => {
      if (mounted.current) setIndex(data.plugins ?? []);
    }).catch(() => {});
  }
  function canBrowserLogin(id: string) {
    return plugins.some(plugin => plugin.id === id && plugin.manifest.capabilities?.includes('browser-login'));
  }
  useEffect(() => {
    mounted.current = true;
    const abort = new AbortController();
    request<{plugins: Plugin[]}>('/api/source-plugins', {signal: abort.signal}).then(data => setPlugins(data.plugins ?? [])).catch(error => {if (!abort.signal.aborted) {setFailed(true); setMessage(String(error));}});
    request<{accounts: LegacyAccount[]}>('/api/source-plugins/legacy-trae', {signal: abort.signal}).then(data => setLegacy(data.accounts ?? [])).catch(() => {});
    void loadPackages(abort.signal);
    void loadIndex(abort.signal);
    return () => {mounted.current = false; abort.abort(); controller.current?.abort();};
  }, []);
  async function run(url: string, body: Record<string, unknown>, success: string) {
    if (controller.current) return false;
    const abort = new AbortController(); controller.current = abort;
    setBusy(true); setMessage(''); setFailed(false); setRetryLogin('');
    if (body.method === 'login/browser') setAuthorizing(url.split('/')[3]);
    try {
      const response = await request<{value?: unknown; display?: PluginDisplay}>(url, {method: 'POST', body: JSON.stringify(body), signal: abort.signal});
      const data = await request<{plugins: Plugin[]}>('/api/source-plugins', {signal: abort.signal});
      if (mounted.current && /\/(install|update)$/.test(url)) {setInstallOpen(false); setPath('');}
      if (mounted.current) {
        setPlugins(data.plugins ?? []);
        setMessage(success);
        if (/\/(install|update|uninstall)$/.test(url)) {void loadPackages(abort.signal); void loadIndex(abort.signal);}
        if (body.method === 'login/browser' || url.endsWith('/legacy-trae')) setResults({});
        if (typeof body.method === 'string' && typeof body.accountId === 'string' && response.value != null) {
          const method = body.method;
          const key = `${url.split('/')[3]}:${body.accountId}:${method === 'quota/get' ? 'quota' : 'checkin'}`;
          setResults(previous => ({...previous, [key]: {method, value: response.value, display: response.display}}));
        }
        modelCatalogChanged();
      }
      return true;
    } catch (error) {
      if (mounted.current) {
        setFailed(!abort.signal.aborted);
        setMessage(abort.signal.aborted ? '授权已取消' : error instanceof Error ? error.message : String(error));
        if (error instanceof ApiError && error.code === 'auth_expired') {
          const id = url.split('/')[3];
          if (canBrowserLogin(id)) setRetryLogin(`/api/source-plugins/${id}`);
        }
      }
      return false;
    }
    finally {controller.current = null; if (mounted.current) {setBusy(false); setAuthorizing('');}}
  }
  async function updatePackage(item: LocalPackage) {
    if (controller.current) return;
    const abort = new AbortController(); controller.current = abort;
    setBusy(true); setMessage(''); setFailed(false);
    let disabled = false;
    try {
      const url = `/api/source-plugins/${item.manifest.id}`;
      if (item.enabled) {
        await request(`${url}/enabled`, {method: 'POST', body: JSON.stringify({enabled: false}), signal: abort.signal});
        disabled = true;
      }
      await request('/api/source-plugins/update', {method: 'POST', body: JSON.stringify({path: item.path}), signal: abort.signal});
      let reenabled = false;
      if (disabled) {
        try {
          await request(`${url}/enabled`, {method: 'POST', body: JSON.stringify({enabled: true}), signal: abort.signal});
          reenabled = true;
        } catch {
          // The update succeeded, but the plugin stays disabled until the user retries.
        }
      }
      const data = await request<{plugins: Plugin[]}>('/api/source-plugins', {signal: abort.signal});
      if (mounted.current) {
        setPlugins(data.plugins ?? []);
        setInstallOpen(false); setPath('');
        setFailed(disabled && !reenabled);
        setMessage(disabled && !reenabled ? `已更新到 v${item.manifest.version}，请重新启用插件` : `已更新到 v${item.manifest.version}`);
      }
      modelCatalogChanged();
      await loadPackages(abort.signal);
      await loadIndex(abort.signal);
    } catch (error) {
      if (mounted.current) {
        setFailed(!abort.signal.aborted);
        setMessage(abort.signal.aborted ? '更新已取消' : disabled ? `更新失败，插件已停用：${String(error)}` : String(error));
      }
      await loadPackages();
      await loadIndex();
    } finally {controller.current = null; if (mounted.current) setBusy(false);}
  }
  const packageItems = packages.map(item => {
    const current = plugins.find(plugin => plugin.id === item.manifest.id);
    return current ? {...item, installedVersion: current.manifest.version, enabled: current.enabled} : item;
  });
  return <section className="plugin-section" aria-label="模型来源插件">
    <div className="plugin-heading">
      <h2>模型来源插件</h2>
      <button type="button" className="app-modal-button secondary" aria-expanded={installOpen} aria-controls="plugin-install" onClick={() => setInstallOpen(!installOpen)}>
        <FolderPlus size={15}/>安装插件
      </button>
    </div>
    <div id="plugin-install" className="plugin-install" hidden={!installOpen}>
      <PluginIndex items={index} busy={busy}
        onInstall={item => void run('/api/source-plugins/install', {path: item.resolvedPath}, `已安装 ${item.name} v${item.version}，请核对后启用`)}
        onUpdate={item => void updatePackage({path: item.resolvedPath, manifest: {id: item.id, name: item.name, version: item.version, entrypoint: ''}, installedVersion: item.installedVersion, updateAvailable: true, enabled: item.enabled})}/>
      <PluginPackages packages={packageItems} busy={busy}
        onInstall={item => void run('/api/source-plugins/install', {path: item.path}, `已安装 ${item.manifest.name} v${item.manifest.version}，请核对后启用`)}
        onUpdate={item => void updatePackage(item)}
        onRefresh={() => void loadPackages()}/>
      <form className="plugin-install-form" onSubmit={event => {event.preventDefault(); void run('/api/source-plugins/install', {path: path.trim()}, '插件已安装，请核对后启用');}}>
        <label>或手动填写插件包目录<input value={path} onChange={event => setPath(event.target.value)} placeholder="包含 manifest.json 的完整路径" required disabled={busy}/></label>
        <div className="source-actions">
          <button className="app-modal-button primary" disabled={busy || !path.trim()}>安装</button>
          <button type="button" className="app-modal-button secondary" disabled={busy || !path.trim()} onClick={() => void run('/api/source-plugins/update', {path: path.trim()}, '插件已更新，请重新启用')}>更新已安装插件</button>
        </div>
        <p>插件以当前用户权限运行，请只安装可信代码。安装时不会执行插件。</p>
      </form>
    </div>
    {!plugins.length && <div className="plugin-empty">安装本地插件包后，可在这里管理账号和模型。</div>}
    {plugins.map(plugin => {
      const url = `/api/source-plugins/${plugin.id}`;
      const capabilities = plugin.manifest.capabilities ?? [];
            const hostLogin = capabilities.includes('host-login');
      const canAuthorize = hostLogin || capabilities.includes('browser-login');
      const canModels = capabilities.includes('models');
      const available = packageItems.find(item => item.manifest.id === plugin.id && item.updateAvailable);
      const act = (method: string, success: string) => void run(`${url}/action`, {method}, success);
      return <article className="plugin-card" key={plugin.id} aria-label={plugin.manifest.name}>
        <header className="plugin-card-header">
          <div className="plugin-identity">
            <span className="plugin-icon"><Box size={19}/></span>
            <div><h3>{plugin.manifest.name}</h3>
              <div className="plugin-submeta">
                <span className="plugin-version">v{plugin.manifest.version}</span>
                <span className="plugin-submeta-label">本地插件</span>
                {available && !plugin.enabled && <button type="button" className="plugin-update-badge" onClick={() => setInstallOpen(true)}>可更新 v{available.manifest.version}</button>}
              </div>
            </div>
          </div>
          <label className="plugin-switch">
            <span>{plugin.enabled ? '已启用' : '已停用'}</span>
            <input type="checkbox" role="switch" aria-label={`启用 ${plugin.manifest.name}`} checked={plugin.enabled} disabled={busy} onChange={() => void run(`${url}/enabled`, {enabled: !plugin.enabled}, plugin.enabled ? '插件已停用' : '插件已启用')}/>
            <span className="plugin-switch-track" aria-hidden="true"/>
          </label>
        </header>
        {plugin.enabled && <div className="plugin-body">
          {available && <div className="plugin-update-note">
            <div><strong>发现 v{available.manifest.version}</strong><span>{canAuthorize ? '更新会短暂停用插件，完成后自动恢复启用。' : '当前版本不支持浏览器授权和旧账号迁移，更新后可添加账号。'}</span></div>
            <button type="button" className="app-modal-button secondary" disabled={busy} onClick={() => void updatePackage(available)}>更新</button>
          </div>}
          <div className="plugin-columns">
            <PluginAccounts accounts={plugin.accounts ?? []} busy={busy} authorizing={authorizing === plugin.id}
              canAuthorize={canAuthorize}
              canQuota={capabilities.includes('quota')}
              canCheckin={capabilities.includes('checkin')}
              legacy={plugin.id === 'xiaotao.trae' ? legacy : []}
              quota={id => results[`${plugin.id}:${id}:quota`]?.value}
              checkin={id => {const result = results[`${plugin.id}:${id}:checkin`]; return result && !result.display && <ResultView result={result}/>;}}
              display={id => results[`${plugin.id}:${id}:quota`]?.display}
              checkinDisplay={id => results[`${plugin.id}:${id}:checkin`]?.display}
              onImport={id => void run('/api/source-plugins/legacy-trae', {id}, '旧账号已加入插件')}
              onAdd={() => act(hostLogin ? 'login/host' : 'login/browser', '账号授权成功')}
              onCancel={() => controller.current?.abort()}
              onOrder={ids => void run(`${url}/account-order`, {ids}, '账号顺序已保存')}
              onAction={(accountId, method, success) => void run(`${url}/action`, {accountId, method}, success)}/>
            <PluginCatalog models={plugin.models ?? []} settings={plugin.modelSettings} busy={busy} canRefresh={canModels}
              onRefresh={() => act('models/refresh', '模型目录已刷新')}
              onSave={settings => run(`${url}/models`, {...settings}, '模型设置已保存')}/>
          </div>
          <PluginSettings fields={plugin.manifest.settings ?? []} values={plugin.settingValues ?? {}} busy={busy}
            onSave={values => run(`${url}/settings`, {values}, '插件设置已保存')}/>
        </div>}
        <PluginAdmin plugin={plugin} busy={busy}
          onUninstall={() => void run(`${url}/uninstall`, {}, '插件已卸载，账号和凭据已删除')}
          onRollback={() => void run(`${url}/rollback`, {}, '插件版本已回滚，请刷新模型目录')}
          onDiagnostics={enabled => void run(`${url}/diagnostics`, {enabled}, enabled ? '已开启诊断日志' : '已关闭诊断日志')}/>
      </article>;
    })}
    {(message || busy) && <div className="plugin-feedback-row">
      <p className={failed ? 'plugin-feedback is-error' : 'plugin-feedback'} role={failed ? 'alert' : 'status'}>{busy ? '处理中…' : message}</p>
      {retryLogin && !busy && <button type="button" className="app-modal-button secondary" onClick={() => {const url = retryLogin; setRetryLogin(''); void run(`${url}/action`, {method: 'login/browser'}, '账号授权成功');}}>重新登录</button>}
    </div>}
  </section>;
}
